all: setup Goo.framework

Goo.framework: goo.go
	gomobile bind -target=ios

.setup-done:
	go get golang.org/x/mobile/cmd/gomobile
	which gomobile > /dev/null || ( echo Could not find installed stuff. Is '$$HOME/go/bin' in your PATH\? ; false )
	gomobile init
	touch .setup-done

setup: .setup-done

clean:
	-rm -fr Goo.framework
