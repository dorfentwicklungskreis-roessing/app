plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
    // Liest app/google-services.json und legt daraus die Firebase-Kennungen
    // als Ressourcen an. Die Datei enthält nur Projekt- und App-Kennungen
    // sowie den öffentlichen API-Schlüssel — kein Geheimnis. Der private
    // Schlüssel für den Versand liegt ausschließlich im Cluster.
    alias(libs.plugins.google.services)
}

// Konfigurierbar per Gradle-Property oder Env, damit CI/E2E andere Werte setzen können.
fun prop(name: String, default: String): String =
    (project.findProperty(name) as String?) ?: System.getenv(name.uppercase().replace(".", "_")) ?: default

android {
    namespace = "de.roessing.app"
    compileSdk = 35

    defaultConfig {
        applicationId = "de.roessing.app"
        minSdk = 26
        targetSdk = 35
        // Bewusst fest und von Hand hochgezählt: zu jedem versionCode gehört ein
        // Änderungshinweis unter store/metadata/android/*/changelogs/<code>.txt,
        // den store/check_metadata.py vor jedem Release einfordert. Eine aus der
        // CI-Laufnummer abgeleitete Nummer ließe sich damit nicht belegen.
        // Muss über 1000103 liegen: Die verteilte 0.1.3 trug diese Nummer
        // (aus einer inzwischen entfernten Automatik). Android installiert
        // keinen Build mit kleinerer Nummer über einen größeren — sonst
        // müssten alle Tester die App erst deinstallieren. Ab hier wieder in
        // Einerschritten weiterzählen.
        versionCode = 1000109
        versionName = "0.1.9"

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"

        buildConfigField("String", "API_BASE_URL", "\"${prop("apiBaseUrl", "https://app.xn--rssing-wxa.de")}\"")
        // Die Website liefert die Veranstaltungen als /events.json.
        buildConfigField(
            "String",
            "WEBSITE_BASE_URL",
            "\"${prop("websiteBaseUrl", "https://xn--rssing-wxa.de")}\"",
        )
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

            // Native Debug-Symbole ins AAB legen (BUNDLE-METADATA/…/debugsymbols/),
            // damit die Play Console Abstürze in nativem Code mit Funktionsnamen
            // statt mit Speicheradressen anzeigt.
            //
            // Gewählt: SYMBOL_TABLE. Gemessen wurde beides mit `bundleRelease`
            // (AGP 8.7.3, NDK 27.0.12077973); die AAB-Größe war in allen drei
            // Fällen identisch 18.869.701 Bytes — ohne die Einstellung, mit
            // SYMBOL_TABLE und mit FULL. Grund: alle mitgelieferten .so-Dateien
            // (libmaplibre.so, libandroidx.graphics.path.so,
            // libdatastore_shared_counter.so) kommen bereits vollständig
            // gestrippt aus ihren AARs — weder .symtab noch .debug_*. AGP meldet
            // dazu je Datei „Unable to extract native debug metadata … because
            // the native debug metadata has already been stripped." und schreibt
            // nichts ins Bundle. Die Einstellung ist heute also wirkungslos, aber
            // kostenlos; sobald eine Abhängigkeit ungestrippte Bibliotheken
            // liefert oder eigener nativer Code dazukommt, greift sie von selbst.
            // SYMBOL_TABLE statt FULL, weil dann nur die Symboltabelle (lesbare
            // Stapelspuren) statt zusätzlicher DWARF-Daten ins Bundle wandert.
            //
            // Für MapLibre selbst gibt es Symbole nur außerhalb von Maven:
            // https://github.com/maplibre/maplibre-native/releases/tag/android-v11.7.1
            // („debug-symbols-maplibre-android-opengl-…tar.gz"). Die lassen sich
            // von Hand in der Play Console unter „App-Bundle-Explorer →
            // Downloads → Assets" zum jeweiligen Bundle nachreichen. Bewusst
            // nicht automatisiert: das Archiv ist ~207 MB und müsste bei jedem
            // Release-Build geladen werden.
            ndk { debugSymbolLevel = "SYMBOL_TABLE" }
        }
    }

    lint {
        // NullSafeMutableLiveData ist in der ausgelieferten Lint-Fassung selbst
        // defekt: der Prüfer stürzt mit
        // „IncompatibleClassChangeError: NonNullableMutableLiveDataDetector
        // $createUastHandler$1.visitCallExpression" ab. Der Fehler steckt im
        // Prüfer, nicht in unserem Code — Lint schlägt in derselben Ausgabe
        // selbst vor, ihn abzuschalten. Weil `bundleRelease` die Aufgabe
        // `lintVitalRelease` mitzieht, riss der Absturz den kompletten
        // Release-Build mit: Lauf 31788054089 (Tag v0.1.6) scheiterte daran im
        // Schritt „Release bauen (APK + AAB)".
        //
        // Bewusst nur dieser eine Prüfer und nicht `abortOnError = false` oder
        // `checkReleaseBuilds = false`: alle übrigen Prüfungen sollen den
        // Release weiterhin blockieren können. Sobald eine neuere AGP-/Lint-
        // Fassung den Prüfer repariert, kann die Zeile wieder weg.
        disable += "NullSafeMutableLiveData"
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions {
        jvmTarget = "17"
        // Material 3 Expressive ist in material3 1.4.0 stabil ausgeliefert, die
        // Signaturen sind aber noch als „experimentell" markiert (der Vertrag
        // kann sich in 1.5 ändern). Wir nutzen daraus bewusst nur wenige
        // Bausteine: MaterialExpressiveTheme, MotionScheme und die
        // ShortNavigationBar. Die Zustimmung steht hier zentral, damit sie in
        // einer Datei sichtbar ist statt in jeder Oberfläche einzeln.
        freeCompilerArgs += "-opt-in=androidx.compose.material3.ExperimentalMaterial3ExpressiveApi"
    }
    buildFeatures {
        compose = true
        buildConfig = true
    }
    testOptions {
        unitTests {
            isIncludeAndroidResources = true
            // Ohne das wirft jeder Aufruf von android.util.Log im Unit-Test
            // eine RuntimeException — auch aus reiner Ablauflogik heraus, die
            // mit Android sonst nichts zu tun hat.
            isReturnDefaultValues = true
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

    implementation(platform(libs.firebase.bom))
    implementation(libs.firebase.messaging)

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
