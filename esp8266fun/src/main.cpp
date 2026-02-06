#include <Arduino.h>
#include <ESP8266WiFi.h>
#include <ESP8266WebServer.h>
#include <ESP8266mDNS.h>
#include <ArduinoOTA.h>
#include <SPI.h>
#include <ELECHOUSE_CC1101_SRC_DRV.h>
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
const int PIN_CC1101_SCK = 14;  // D5
const int PIN_CC1101_MISO = 12; // D6
const int PIN_CC1101_MOSI = 13; // D7
const int PIN_CC1101_CSN = 5;   // D1 (moved from D8)
const int PIN_CC1101_GDO0 = 4;  // D2
const int PIN_RELAY = 16;       // D0 (GPIO16) - no boot strap, no interrupt

const bool RELAY_ACTIVE_LOW = true;

struct ScanHit {
    float mhz;
    int rssi;
    unsigned long seenMs;
};

const int MAX_SCAN_HITS = 12;
ScanHit scanHits[MAX_SCAN_HITS];
int scanHitCount = 0;

bool cc1101Ready = false;
bool scanActive = false;
bool captureActive = false;
float scanStartMhz = 318.0f;
float scanStopMhz = 318.6f;
float scanStepMhz = 0.1f;
int scanThreshold = -85;
unsigned long scanDwellMs = 40;
float scanCurrentMhz = 0.0f;
unsigned long lastScanStep = 0;
int lastScanRssi = 0;

const int MAX_PULSES = 400;
volatile unsigned long pulseDurations[MAX_PULSES];
volatile int pulseCount = 0;
volatile unsigned long lastEdgeMicros = 0;
float captureFreqMhz = 318.0f;

void ICACHE_RAM_ATTR onGdo0Change() {
    unsigned long now = micros();
    unsigned long delta = now - lastEdgeMicros;
    lastEdgeMicros = now;
    if (pulseCount < MAX_PULSES) {
        pulseDurations[pulseCount++] = delta;
    }
}

bool isPinInverted(int pin) {
    if (pin == LED_BUILTIN) {
        return true;
    }
    if (pin == PIN_RELAY && RELAY_ACTIVE_LOW) {
        return true;
    }
    return false;
}

void writePin(int pin, int value) {
    if (isPinInverted(pin)) {
        digitalWrite(pin, value ? LOW : HIGH);
    } else {
        digitalWrite(pin, value ? HIGH : LOW);
    }
}

int readPin(int pin) {
    int v = digitalRead(pin);
    if (isPinInverted(pin)) {
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
        {PIN_RELAY, "Relay (GPIO16 / D0)", false},
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

    html += "<div class=\"row\"><div>CC1101 Scanner</div><div>";
    html += "<a class=\"btn\" href=\"/cc1101\">Open</a>";
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

    if (pin == PIN_LED || pin == PIN_RELAY) {
        pinMode(pin, OUTPUT);
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

void addScanHit(float mhz, int rssi) {
    if (scanHitCount < MAX_SCAN_HITS) {
        scanHits[scanHitCount++] = {mhz, rssi, millis()};
        return;
    }
    int oldestIndex = 0;
    for (int i = 1; i < MAX_SCAN_HITS; i++) {
        if (scanHits[i].seenMs < scanHits[oldestIndex].seenMs) {
            oldestIndex = i;
        }
    }
    scanHits[oldestIndex] = {mhz, rssi, millis()};
}

void handleCC1101() {
    if (!ensureAuth()) {
        return;
    }

    String html;
    html += "<!doctype html><html><head>";
    html += "<meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">";
    html += "<title>CC1101 Scanner</title>";
    html += "<style>";
    html += "body{font-family:sans-serif;background:#f5f5f7;margin:0;padding:20px;}";
    html += "h1{margin:0 0 8px;}";
    html += ".card{background:#fff;border-radius:12px;padding:16px;box-shadow:0 2px 8px rgba(0,0,0,0.08);max-width:720px;}";
    html += "label{display:block;margin:10px 0 4px;font-size:12px;color:#444;}";
    html += "input{width:100%;padding:8px;border:1px solid #ddd;border-radius:6px;}";
    html += ".row{display:flex;gap:10px;}";
    html += ".row>div{flex:1;}";
    html += ".btn{padding:10px 14px;border:0;border-radius:8px;background:#0070f3;color:white;cursor:pointer;text-decoration:none;display:inline-block;margin-top:10px;}";
    html += ".btn.off{background:#666;}";
    html += "pre{background:#111;color:#0f0;padding:10px;border-radius:8px;overflow:auto;}";
    html += ".note{font-size:12px;color:#666;margin-top:10px;}";
    html += "</style></head><body>";
    html += "<div class=\"card\">";
    html += "<h1>CC1101 Scanner</h1>";
    html += "<p class=\"note\">RX-only scan for energy bursts. Rolling-code remotes may not be clonable.</p>";

    html += "<div class=\"row\">";
    html += "<div><label>Start MHz</label><input id=\"start\" type=\"number\" step=\"0.01\" value=\"318.0\"></div>";
    html += "<div><label>Stop MHz</label><input id=\"stop\" type=\"number\" step=\"0.01\" value=\"318.6\"></div>";
    html += "</div>";
    html += "<div class=\"row\">";
    html += "<div><label>Step MHz</label><input id=\"step\" type=\"number\" step=\"0.01\" value=\"0.10\"></div>";
    html += "<div><label>RSSI threshold (dBm)</label><input id=\"threshold\" type=\"number\" step=\"1\" value=\"-85\"></div>";
    html += "</div>";
    html += "<div class=\"row\">";
    html += "<div><label>Dwell (ms per step)</label><input id=\"dwell\" type=\"number\" step=\"1\" value=\"40\"></div>";
    html += "<div></div>";
    html += "</div>";

    html += "<button class=\"btn\" onclick=\"startScan()\">Start scan</button> ";
    html += "<button class=\"btn off\" onclick=\"stopScan()\">Stop scan</button>";
    html += "<button class=\"btn\" onclick=\"startCapture()\">Start capture</button> ";
    html += "<button class=\"btn off\" onclick=\"stopCapture()\">Stop capture</button>";
    html += "<button class=\"btn off\" onclick=\"clearCapture()\">Clear pulses</button>";
    html += "<div class=\"note\" id=\"cc1101Ready\">CC1101 init: checking...</div>";
    html += "<div class=\"note\" id=\"cc1101Rssi\">RSSI: -- dBm</div>";
    html += "<pre id=\"status\">Loading...</pre>";
    html += "<div class=\"note\">Pins: SCK D5, MISO D6, MOSI D7, CSn D1, GDO0 D2. Use 3.3V only.</div>";
    html += "</div>";

    html += "<script>";
    html += "async function req(url){";
    html += "const res=await fetch(url,{credentials:'same-origin',cache:'no-store'});";
    html += "if(!res.ok){throw new Error('HTTP '+res.status);} return res;}";
    html += "async function startScan(){";
    html += "const p=new URLSearchParams({start:val('start'),stop:val('stop'),step:val('step'),threshold:val('threshold'),dwell:val('dwell')});";
    html += "await req('/cc1101/start?'+p.toString());}";
    html += "async function stopScan(){await req('/cc1101/stop');}";
    html += "async function startCapture(){";
    html += "const p=new URLSearchParams({freq:val('start')});";
    html += "await req('/cc1101/capture/start?'+p.toString());}";
    html += "async function stopCapture(){await req('/cc1101/capture/stop');}";
    html += "async function clearCapture(){await req('/cc1101/capture/clear');}";
    html += "function val(id){return document.getElementById(id).value;}";
    html += "async function poll(){";
    html += "try{";
    html += "const freq=val('start');";
    html += "const [scanRes,captureRes,rssiRes]=await Promise.all([req('/cc1101/status'),req('/cc1101/capture/status'),req('/cc1101/rssi?freq='+encodeURIComponent(freq))]);";
    html += "const scan=await scanRes.json();";
    html += "const capture=await captureRes.json();";
    html += "const rssi=await rssiRes.json();";
    html += "document.getElementById('cc1101Ready').textContent='CC1101 init: '+(scan.ready?'OK':'FAILED');";
    html += "document.getElementById('cc1101Rssi').textContent='RSSI: '+rssi.rssi+' dBm @ '+rssi.freq_mhz+' MHz';";
    html += "document.getElementById('status').textContent=JSON.stringify({scan,capture},null,2);";
    html += "}catch(err){";
    html += "document.getElementById('status').textContent='Error: '+err.message;";
    html += "}";
    html += "}";
    html += "setInterval(poll,1000);poll();";
    html += "</script></body></html>";

    server.send(200, "text/html", html);
}

void handleCC1101Start() {
    if (!ensureAuth()) {
        return;
    }
    if (!cc1101Ready) {
        server.send(500, "text/plain", "CC1101 not initialized");
        return;
    }

    if (server.hasArg("start")) {
        scanStartMhz = server.arg("start").toFloat();
    }
    if (server.hasArg("stop")) {
        scanStopMhz = server.arg("stop").toFloat();
    }
    if (server.hasArg("step")) {
        scanStepMhz = server.arg("step").toFloat();
    }
    if (server.hasArg("threshold")) {
        scanThreshold = server.arg("threshold").toInt();
    }
    if (server.hasArg("dwell")) {
        scanDwellMs = static_cast<unsigned long>(server.arg("dwell").toInt());
    }

    if (scanStepMhz <= 0.0f) {
        scanStepMhz = 0.1f;
    }
    if (scanStopMhz < scanStartMhz) {
        float tmp = scanStartMhz;
        scanStartMhz = scanStopMhz;
        scanStopMhz = tmp;
    }

    scanHitCount = 0;
    scanCurrentMhz = scanStartMhz;
    scanActive = true;
    lastScanStep = 0;
    server.send(200, "application/json", "{\"ok\":true}");
}

void handleCC1101Stop() {
    if (!ensureAuth()) {
        return;
    }
    scanActive = false;
    server.send(200, "application/json", "{\"ok\":true}");
}

void handleCC1101Status() {
    if (!ensureAuth()) {
        return;
    }

    String json = "{";
    json += "\"ready\":";
    json += cc1101Ready ? "true" : "false";
    json += ",\"active\":";
    json += scanActive ? "true" : "false";
    json += ",\"capture_active\":";
    json += captureActive ? "true" : "false";
    json += ",\"current_mhz\":";
    json += String(scanCurrentMhz, 3);
    json += ",\"last_rssi\":";
    json += lastScanRssi;
    json += ",\"threshold\":";
    json += scanThreshold;
    json += ",\"dwell_ms\":";
    json += scanDwellMs;
    json += ",\"hits\":[";
    for (int i = 0; i < scanHitCount; i++) {
        if (i > 0) {
            json += ",";
        }
        json += "{";
        json += "\"mhz\":";
        json += String(scanHits[i].mhz, 3);
        json += ",\"rssi\":";
        json += scanHits[i].rssi;
        json += ",\"age_ms\":";
        json += (millis() - scanHits[i].seenMs);
        json += "}";
    }
    json += "]";
    json += ",\"pulse_count\":";
    json += pulseCount;
    json += "}";

    server.send(200, "application/json", json);
}

void handleCC1101Rssi() {
    if (!ensureAuth()) {
        return;
    }
    if (!cc1101Ready) {
        server.send(500, "text/plain", "CC1101 not initialized");
        return;
    }
    float mhz = scanCurrentMhz;
    if (server.hasArg("freq")) {
        mhz = server.arg("freq").toFloat();
    }
    ELECHOUSE_cc1101.setMHZ(mhz);
    ELECHOUSE_cc1101.SetRx();
    delay(2);
    int rssi = ELECHOUSE_cc1101.getRssi();
    String json = "{";
    json += "\"freq_mhz\":";
    json += String(mhz, 3);
    json += ",\"rssi\":";
    json += rssi;
    json += "}";
    server.send(200, "application/json", json);
}

void handleCC1101CaptureStart() {
    if (!ensureAuth()) {
        return;
    }
    if (!cc1101Ready) {
        server.send(500, "text/plain", "CC1101 not initialized");
        return;
    }
    if (server.hasArg("freq")) {
        captureFreqMhz = server.arg("freq").toFloat();
    }
    scanActive = false;
    noInterrupts();
    pulseCount = 0;
    lastEdgeMicros = micros();
    interrupts();
    ELECHOUSE_cc1101.setCCMode(0);
    ELECHOUSE_cc1101.setModulation(2); // ASK/OOK
    ELECHOUSE_cc1101.setPktFormat(2);  // async serial
    ELECHOUSE_cc1101.setSyncMode(0);   // no sync word
    ELECHOUSE_cc1101.setCrc(false);
    ELECHOUSE_cc1101.setWhiteData(false);
    ELECHOUSE_cc1101.SpiWriteReg(CC1101_IOCFG0, 0x0D); // GDO0: serial data out
    ELECHOUSE_cc1101.setMHZ(captureFreqMhz);
    ELECHOUSE_cc1101.SetRx();
    pinMode(PIN_CC1101_GDO0, INPUT);
    attachInterrupt(digitalPinToInterrupt(PIN_CC1101_GDO0), onGdo0Change, CHANGE);
    captureActive = true;
    server.send(200, "application/json", "{\"ok\":true}");
}

void handleCC1101CaptureStop() {
    if (!ensureAuth()) {
        return;
    }
    detachInterrupt(digitalPinToInterrupt(PIN_CC1101_GDO0));
    captureActive = false;
    server.send(200, "application/json", "{\"ok\":true}");
}

void handleCC1101CaptureClear() {
    if (!ensureAuth()) {
        return;
    }
    noInterrupts();
    pulseCount = 0;
    interrupts();
    server.send(200, "application/json", "{\"ok\":true}");
}

void handleCC1101CaptureStatus() {
    if (!ensureAuth()) {
        return;
    }
    String json = "{";
    json += "\"active\":";
    json += captureActive ? "true" : "false";
    json += ",\"freq_mhz\":";
    json += String(captureFreqMhz, 3);
    json += ",\"pulse_count\":";
    json += pulseCount;
    json += ",\"pulses\":[";

    int countCopy = pulseCount;
    if (countCopy > MAX_PULSES) {
        countCopy = MAX_PULSES;
    }
    int startIndex = countCopy > 80 ? countCopy - 80 : 0;
    for (int i = startIndex; i < countCopy; i++) {
        if (i > startIndex) {
            json += ",";
        }
        json += pulseDurations[i];
    }
    json += "]}";
    server.send(200, "application/json", json);
}

void setup() {
    Serial.begin(115200);
    delay(100);

    Serial.println("\n\nESP8266 WiFi Connection Test");
    Serial.println("============================");

    SPI.begin();
    ELECHOUSE_cc1101.setSpiPin(PIN_CC1101_SCK, PIN_CC1101_MISO, PIN_CC1101_MOSI, PIN_CC1101_CSN);
    ELECHOUSE_cc1101.Init();
    ELECHOUSE_cc1101.setGDO0(PIN_CC1101_GDO0);
    cc1101Ready = ELECHOUSE_cc1101.getCC1101();
    if (cc1101Ready) {
        ELECHOUSE_cc1101.SetRx();
        Serial.println("CC1101 initialized");
    } else {
        Serial.println("CC1101 init failed");
    }

    pinMode(PIN_LED, OUTPUT);
    writePin(PIN_LED, 1);

    pinMode(PIN_RELAY, OUTPUT);
    writePin(PIN_RELAY, 0);

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
        server.on("/cc1101", handleCC1101);
        server.on("/cc1101/start", handleCC1101Start);
        server.on("/cc1101/stop", handleCC1101Stop);
        server.on("/cc1101/status", handleCC1101Status);
        server.on("/cc1101/rssi", handleCC1101Rssi);
        server.on("/cc1101/capture/start", handleCC1101CaptureStart);
        server.on("/cc1101/capture/stop", handleCC1101CaptureStop);
        server.on("/cc1101/capture/clear", handleCC1101CaptureClear);
        server.on("/cc1101/capture/status", handleCC1101CaptureStatus);
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
        if (scanActive && cc1101Ready && !captureActive) {
            unsigned long now = millis();
            if (now - lastScanStep >= scanDwellMs) {
                ELECHOUSE_cc1101.setMHZ(scanCurrentMhz);
                ELECHOUSE_cc1101.SetRx();
                delay(2);
                lastScanRssi = ELECHOUSE_cc1101.getRssi();
                if (lastScanRssi >= scanThreshold) {
                    addScanHit(scanCurrentMhz, lastScanRssi);
                }
                scanCurrentMhz += scanStepMhz;
                if (scanCurrentMhz > scanStopMhz) {
                    scanActive = false;
                }
                lastScanStep = now;
            }
        }
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
