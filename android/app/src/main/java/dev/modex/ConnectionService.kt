package dev.modex

import android.app.*
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder
import androidx.lifecycle.ViewModelStore
import androidx.lifecycle.ViewModelStoreOwner

class ModexApplication : Application(), ViewModelStoreOwner {
    var session: ModexViewModel? = null
    override val viewModelStore = ViewModelStore()
}

class ConnectionService : Service() {
    override fun onCreate() {
        super.onCreate()
        val manager = getSystemService(NotificationManager::class.java)
        manager.createNotificationChannel(NotificationChannel("connection", "Host connection", NotificationManager.IMPORTANCE_LOW))
        val open = PendingIntent.getActivity(this, 0, Intent(this, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        val disconnect = PendingIntent.getService(this, 1, Intent(this, ConnectionService::class.java).setAction("disconnect"), PendingIntent.FLAG_IMMUTABLE)
        val notification = Notification.Builder(this, "connection").setSmallIcon(android.R.drawable.stat_notify_sync)
            .setContentTitle("Modex host connection").setContentText("Your host keeps working. Tap to return to chat.")
            .setContentIntent(open).setOngoing(true).addAction(Notification.Action.Builder(null, "Disconnect", disconnect).build()).build()
        if (Build.VERSION.SDK_INT >= 29) startForeground(1, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC)
        else startForeground(1, notification)
    }
    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == "disconnect") (application as ModexApplication).session?.disconnect()
        return START_NOT_STICKY
    }
    override fun onBind(intent: Intent?): IBinder? = null
    override fun onTimeout(startId: Int, fgsType: Int) { (application as ModexApplication).session?.disconnect(); stopSelf() }
}
