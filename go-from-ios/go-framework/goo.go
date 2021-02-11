package goo

import (
	"fmt"
	"net"
	"net/http"
)

func Inc(i int) int {
	return i + 1
}

func Greetings(name string) string {
	go func() {
		http.HandleFunc("/", handler)
		http.ListenAndServe(":8080", nil)
	}()
	return fmt.Sprintf("Hallo, %s! My addresses are %s", name, getMyIPs())
}

func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hi there, I love %s!", r.URL.Path[1:])
}

func getMyIPs() string {
	ifaces, err := net.Interfaces()
	s := ""
	if err != nil {
		return "unknown"
	}
	for _, i := range ifaces {
		addrs, err := i.Addrs()
		if err != nil {
			return "huh"
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if s != "" {
				s += ", " + ip.String()
			} else {
				s = ip.String()
			}
		}
	}
	return s
}
