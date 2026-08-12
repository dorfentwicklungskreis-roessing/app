# Retrofit/kotlinx.serialization: Modelle behalten
-keepattributes *Annotation*, InnerClasses, Signature
-keep,includedescriptorclasses class de.roessing.app.data.**$$serializer { *; }
-keepclassmembers class de.roessing.app.data.** {
    *** Companion;
}
-keepclasseswithmembers class de.roessing.app.data.** {
    kotlinx.serialization.KSerializer serializer(...);
}
# AppAuth nutzt Reflection auf Browser-Pakete nicht — nichts nötig.
# MapLibre: native Bibliothek, keine zusätzlichen Regeln erforderlich.
