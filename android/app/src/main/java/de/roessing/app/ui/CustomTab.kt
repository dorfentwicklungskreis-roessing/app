package de.roessing.app.ui

import android.content.Context
import android.content.Intent
import android.net.Uri
import androidx.browser.customtabs.CustomTabsIntent

/**
 * Opens a page in the in-app browser (Custom Tab). If the device has none, a
 * regular browser will do; if there is no browser either, nothing happens —
 * a crash would be the worst possible answer here.
 *
 * Shared by every place in the app that points at a web page: the legal
 * pages and the administration hint.
 */
internal fun Context.openInCustomTab(url: String) {
    val target = Uri.parse(url)
    val tab = CustomTabsIntent.Builder().setShowTitle(true).build()
    tab.intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
    runCatching { tab.launchUrl(this, target) }.recoverCatching {
        startActivity(Intent(Intent.ACTION_VIEW, target).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
    }
}
