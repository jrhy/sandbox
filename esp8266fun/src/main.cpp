#include <Arduino.h>
#include <ESP8266WiFi.h>
#include <ESP8266WebServer.h>
#include <ESP8266mDNS.h>
#include <ArduinoOTA.h>
#include "credentials.h"

#ifndef WEB_USER
#define WEB_USER ""
#endif

#ifndef WEB_PASS
#define WEB_PASS ""
#endif

#ifndef OTA_PASS
#define OTA_PASS ""
#endif

#ifndef OTA_HOSTNAME
#define OTA_HOSTNAME ""
#endif

ESP8266WebServer server(80);

const int PIN_LED = LED_BUILTIN; // GPIO2 on many ESP8266 boards; inverted
const int PIN_GPIO0 = 0;
const int PIN_GPIO2 = 2;

bool isLedInverted(int pin) {
    return pin == LED_BUILTIN;
}

void writePin(int pin, int value) {
    if (isLedInverted(pin)) {
        digitalWrite(pin, value ? LOW : HIGH);
    } else {
        digitalWrite(pin, value ? HIGH : LOW);
    }
}

int readPin(int pin) {
    int v = digitalRead(pin);
    if (isLedInverted(pin)) {
        return v == LOW ? 1 : 0;
    }
    return v == HIGH ? 1 : 0;
}

bool unsafeEnabled() {
    return server.hasArg("unsafe") && server.arg("unsafe") == "1";
}

bool hasWebAuth() {
    return WEB_USER[0] != '\0' && WEB_PASS[0] != '\0';
}

bool ensureAuth() {
    if (!hasWebAuth()) {
        return true;
    }
    if (server.authenticate(WEB_USER, WEB_PASS)) {
        return true;
    }
    server.requestAuthentication();
    return false;
}

void handleRoot() {
    if (!ensureAuth()) {
        return;
    }

    const bool unsafe = unsafeEnabled();

    String html;
    html += "<!doctype html><html><head>";
    html += "<meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">";
    html += "<title>ESP8266 GPIO Control</title>";
    html += "<style>";
    html += "body{font-family:sans-serif;background:#f5f5f7;margin:0;padding:20px;}";
    html += "h1{margin:0 0 8px;}";
    html += "p{margin:0 0 16px;color:#444;}";
    html += ".card{background:#fff;border-radius:12px;padding:16px;box-shadow:0 2px 8px rgba(0,0,0,0.08);max-width:560px;}";
    html += ".row{display:flex;align-items:center;justify-content:space-between;padding:10px 0;border-bottom:1px solid #eee;}";
    html += ".row:last-child{border-bottom:none;}";
    html += ".btn{padding:10px 14px;border:0;border-radius:8px;background:#0070f3;color:white;cursor:pointer;text-decoration:none;display:inline-block;}";
    html += ".btn.off{background:#666;}";
    html += ".btn.disabled{background:#ccc;cursor:not-allowed;pointer-events:none;}";
    html += ".note{font-size:12px;color:#666;margin-top:10px;}";
    html += "</style></head><body>";
    html += "<div class=\"card\">";
    html += "<h1>ESP8266 GPIO Control</h1>";
    html += "<p>GPIO0 and GPIO2 are boot-critical on many ESP8266 boards. They are disabled by default.</p>";

    struct PinInfo { int pin; const char* name; bool unsafeOnly; } pins[] = {
        {PIN_LED, "LED (GPIO2)", false},
        {PIN_GPIO0, "GPIO0", true},
        {PIN_GPIO2, "GPIO2", true}
    };

    for (auto &p : pins) {
        const bool isUnsafePin = p.unsafeOnly && !unsafe;
        int state = readPin(p.pin);

        html += "<div class=\"row\"><div>";
        html += p.name;
        if (isUnsafePin) {
            html += " <span class=\"note\">(disabled)</span>";
        }
        html += "</div><div>";

        if (isUnsafePin) {
            html += "<span class=\"btn disabled\">Enable to control</span>";
        } else {
            html += "<a class=\"btn";
            html += state ? " off" : "";
            html += "\" href=\"/set?pin=";
            html += p.pin;
            html += "&value=";
            html += state ? "0" : "1";
            if (unsafe) {
                html += "&unsafe=1";
            }
            html += "\">";
            html += state ? "Turn Off" : "Turn On";
            html += "</a>";
        }

        html += "</div></div>";
    }

    html += "<div class=\"row\"><div>Unsafe controls</div><div>";
    if (unsafe) {
        html += "<a class=\"btn off\" href=\"/\">Disable</a>";
    } else {
        html += "<a class=\"btn\" href=\"/?unsafe=1\">Enable</a>";
    }
    html += "</div></div>";

    html += "<div class=\"note\">Tip: avoid toggling GPIO0/2 while booting.</div>";
    html += "</div></body></html>";

    server.send(200, "text/html", html);
}

void handleSet() {
    if (!ensureAuth()) {
        return;
    }

    if (!server.hasArg("pin") || !server.hasArg("value")) {
        server.send(400, "text/plain", "Missing pin or value");
        return;
    }

    int pin = server.arg("pin").toInt();
    int value = server.arg("value").toInt();
    const bool unsafe = unsafeEnabled();

    if (pin == PIN_LED) {
        pinMode(PIN_LED, OUTPUT);
        writePin(pin, value);
    } else if ((pin == PIN_GPIO0 || pin == PIN_GPIO2) && unsafe) {
        pinMode(pin, OUTPUT);
        writePin(pin, value);
    } else {
        server.send(400, "text/plain", "Unsupported or disabled pin");
        return;
    }

    String redirect = "/";
    if (unsafe) {
        redirect += "?unsafe=1";
    }
    server.sendHeader("Location", redirect);
    server.send(303);
}

void setup() {
    Serial.begin(115200);
    delay(100);

    Serial.println("\n\nESP8266 WiFi Connection Test");
    Serial.println("============================");

    pinMode(PIN_LED, OUTPUT);
    writePin(PIN_LED, 0);

    pinMode(PIN_GPIO0, INPUT);
    pinMode(PIN_GPIO2, INPUT);

    WiFi.mode(WIFI_STA);
    WiFi.begin(WIFI_SSID, WIFI_PASS);

    Serial.printf("Connecting to %s", WIFI_SSID);

    int attempts = 0;
    while (WiFi.status() != WL_CONNECTED && attempts < 30) {
        delay(500);
        Serial.print(".");
        attempts++;
    }

    if (WiFi.status() == WL_CONNECTED) {
        Serial.println(" Connected!");
        Serial.printf("IP Address: %s\n", WiFi.localIP().toString().c_str());
        Serial.printf("Signal strength (RSSI): %d dBm\n", WiFi.RSSI());
        Serial.printf("MAC Address: %s\n", WiFi.macAddress().c_str());

        if (OTA_HOSTNAME[0] != '\0') {
            if (MDNS.begin(OTA_HOSTNAME)) {
                Serial.printf("mDNS started: http://%s.local/\n", OTA_HOSTNAME);
            } else {
                Serial.println("mDNS failed to start");
            }
            ArduinoOTA.setHostname(OTA_HOSTNAME);
        }

        if (OTA_PASS[0] != '\0') {
            ArduinoOTA.setPassword(OTA_PASS);
        }

        ArduinoOTA.begin();
        Serial.println("OTA ready");

        if (!hasWebAuth()) {
            Serial.println("WARNING: WEB_USER/WEB_PASS not set; web UI is unauthenticated");
        }

        server.on("/", handleRoot);
        server.on("/set", handleSet);
        server.begin();
        Serial.println("Web server started on port 80");
    } else {
        Serial.println(" Failed!");
        Serial.println("Could not connect to WiFi");
    }
}

void loop() {
    static unsigned long lastLog = 0;

    if (WiFi.status() == WL_CONNECTED) {
        ArduinoOTA.handle();
        MDNS.update();
        server.handleClient();
        if (millis() - lastLog >= 5000) {
            Serial.printf("Connected to %s | IP: %s | RSSI: %d dBm\n",
                WIFI_SSID,
                WiFi.localIP().toString().c_str(),
                WiFi.RSSI()
            );
            lastLog = millis();
        }
    } else {
        Serial.println("WiFi disconnected, attempting reconnect...");
        WiFi.reconnect();
        delay(1000);
    }
}
