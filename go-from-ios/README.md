This Frankensteins the Xcode 12.4's SwiftUI HelloWorld tutorial
to show how to hook up interesting Go code to an iOS app with `gomobile`.

Steps:

* `make` to build the Go server code into an iOS Framework
* cd IOSApp && open HelloWorld.xcodeproj
* Run
* `curl http://localhost:8080/curl` (for simulator)
* Should be able to install on a device too

