plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
}

// Konfigurierbar per Gradle-Property oder Env, damit CI/E2E andere Werte setzen können.
fun prop(name: String, default: String): String =
    (project.findProperty(name) as String?) ?: System.getenv(name.uppercase().replace(".", "_")) ?: default

// Version: kommt im Release aus dem Git-Tag (der Release-Workflow setzt
// APP_VERSION=0.1.3), lokal steht "0.0.0-dev" drin. So zeigt die App genau die
// Version an, die auch getaggt und verteilt wurde.
val appVersion: String = (project.findProperty("appVersion") as String?)
    ?: System.getenv("APP_VERSION")?.removePrefix("v")?.takeIf { it.isNotBlank() }
    ?: "0.0.0-dev"

// versionCode aus der Version: 0.1.3 -> 1000103. Dieselbe Formel steht in
// scripts/naechste_version.py (Funktion version_code); der Release-Workflow
// vergleicht beide Werte, damit sie nicht auseinanderlaufen. Der Sockel von
// 1.000.000 hält Abstand zu den alten Codes (100 + CI-Laufnummer).
val appVersionCode: Int = Regex("""^(\d+)\.(\d+)\.(\d+)""").find(appVersion)
    ?.destructured
    ?.let { (major, minor, patch) ->
        1_000_000 + major.toInt() * 10_000 + minor.toInt() * 100 + patch.toInt()
    }
    ?: 1_000_000

// Für die CI: ./gradlew -q :app:versionInfo gibt beide Werte aus.
tasks.register("versionInfo") {
    doLast {
        println("versionName=$appVersion")
        println("versionCode=$appVersionCode")
    }
}

android {
    namespace = "de.roessing.app"
    compileSdk = 35

    defaultConfig {
        applicationId = "de.roessing.app"
        minSdk = 26
        targetSdk = 35
        // Beides kommt aus dem Tag (oben abgeleitet). Der Änderungshinweis
        // unter store/metadata/android/*/changelogs/<code>.txt, den
        // store/check_metadata.py einfordert, ist damit schon beim Taggen
        // bekannt und wird von scripts/aenderungsnotiz.py erzeugt.
        versionCode = appVersionCode
        versionName = appVersion

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"

        buildConfigField("String", "API_BASE_URL", "\"${prop("apiBaseUrl", "https://app.xn--rssing-wxa.de")}\"")
        buildConfigField("String", "OIDC_ISSUER", "\"${prop("oidcIssuer", "https://id.xn--rssing-wxa.de")}\"")
        buildConfigField("String", "OIDC_CLIENT_ID", "\"${prop("oidcClientId", "385941807986376899")}\"")
        buildConfigField("String", "OIDC_REDIRECT_URI", "\"de.roessing.app:/oauth2redirect\"")

        // AppAuth: Redirect-Scheme für den Browser-Rücksprung.
        manifestPlaceholders["appAuthRedirectScheme"] = "de.roessing.app"
    }

    signingConfigs {
        // Release-Signing aus Env (CI-Secrets); ohne Secrets wird debug-signiert,
        // damit lokale Builds und PR-Artefakte immer funktionieren.
        create("release") {
            val ksFile = System.getenv("KEYSTORE_FILE")
            if (ksFile != null) {
                storeFile = file(ksFile)
                storePassword = System.getenv("KEYSTORE_PASSWORD")
                keyAlias = System.getenv("KEY_ALIAS")
                keyPassword = System.getenv("KEY_PASSWORD")
            }
        }
    }

    buildTypes {
        debug {
            // DEV_AUTH erlaubt den „Entwickler-Login" (ohne Zitadel) für lokale
            // Entwicklung und E2E-Tests gegen ein Backend mit AUTH_MODE=insecure-dev.
            buildConfigField("boolean", "DEV_AUTH", prop("devAuth", "false"))
        }
        release {
            buildConfigField("boolean", "DEV_AUTH", "false")
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            signingConfig = if (System.getenv("KEYSTORE_FILE") != null)
                signingConfigs.getByName("release") else signingConfigs.getByName("debug")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions {
        jvmTarget = "17"
    }
    buildFeatures {
        compose = true
        buildConfig = true
    }
    testOptions {
        unitTests {
            isIncludeAndroidResources = true
        }
    }
}

dependencies {
    implementation(platform(libs.compose.bom))
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.navigation.compose)
    implementation(libs.androidx.lifecycle.runtime.ktx)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(libs.androidx.datastore.preferences)
    implementation(libs.androidx.browser)

    implementation(libs.compose.ui)
    implementation(libs.compose.ui.graphics)
    implementation(libs.compose.ui.tooling.preview)
    implementation(libs.compose.material3)
    implementation(libs.compose.material.icons)

    implementation(libs.kotlinx.coroutines.android)
    implementation(libs.kotlinx.serialization.json)

    implementation(libs.appauth)
    implementation(libs.maplibre)

    implementation(libs.retrofit)
    implementation(libs.retrofit.kotlinx.serialization)
    implementation(libs.okhttp)
    implementation(libs.okhttp.logging)

    debugImplementation(libs.compose.ui.tooling)
    debugImplementation(libs.compose.ui.test.manifest)

    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation(libs.okhttp.mockwebserver)

    androidTestImplementation(platform(libs.compose.bom))
    androidTestImplementation(libs.androidx.junit)
    androidTestImplementation(libs.androidx.espresso.core)
    androidTestImplementation(libs.compose.ui.test.junit4)
    androidTestImplementation(libs.androidx.uiautomator)
}
