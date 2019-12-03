package main

import (
	"flag"
	"fmt"
	oh "github.com/ossrs/go-oryx-lib/http"
	ol "github.com/ossrs/go-oryx-lib/logger"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
)

func GetOriginalClientIP(r *http.Request) string {
	// https://gtranslate.io/forum/http-real-http-forwarded-for-t2980.html
	//current the order to get client ip is clientip > X-Forwarded-For > X-Real-IP > remote addr
	var rip string

	q := r.URL.Query()
	if rip = q.Get("clientip"); rip != "" {
		return rip
	}

	if forwordIP := r.Header.Get("X-Forwarded-For"); forwordIP != "" {
		index := strings.Index(forwordIP, ",")
		if index != -1 {
			rip = forwordIP[:index]
		} else {
			rip = forwordIP
		}
		return rip
	}

	if rip = r.Header.Get("X-Real-IP"); rip == "" {
		if nip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			rip = nip
		}
	}

	return rip
}

func main() {
	var port int
	flag.IntVar(&port, "port", 0, "Listen port.")

	flag.Usage = func() {
		fmt.Println(fmt.Sprintf("HTTP GIF as SLS writer"))
		flag.PrintDefaults()
		fmt.Println(fmt.Sprintf("For example:"))
		fmt.Println(fmt.Sprintf("		%v -port=1987", os.Args[0]))
	}

	flag.Parse()

	if port == 0 {
		flag.Usage()
		os.Exit(-1)
	}

	oh.Server = "go-oryx"

	ol.Tf(nil, "Handle /")
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		rip := GetOriginalClientIP(r)
		ua := r.Header.Get("User-Agent")
		referer := r.Header.Get("Referer")
		url := r.URL.RawQuery
		ol.Tf(nil, "rip=%v, url=%v, referer=%v, ua=%v", rip, url, referer, ua)

		h := w.Header()
		h.Set("Server", "go-oryx")
		h.Set("Content-Type", "image/gif")
		h.Set("Connection", "close")
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Pragma", "no-cache")
		h.Set("Cache-Control", "no-cache, no-store, must-revalidate, max-age=0")
		h.Set("Expires", "0")

		// GIF for HTML img at https://help.aliyun.com/document_detail/31752.html
		b := []byte{
			0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x80, 0x01, 0x00, 0x00, 0x00, 0x00,
			0xff, 0xff, 0xff, 0x21, 0xf9, 0x04, 0x01, 0x00, 0x00, 0x01, 0x00, 0x2c, 0x00, 0x00, 0x00, 0x00,
			0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x02, 0x4c,
		}
		io.Copy(w, strings.NewReader(string(b)))
	})

	// HTML img at https://help.aliyun.com/document_detail/31752.html
	query := "https://xxx/logstores/xxx/track_ua.gif?APIVersion=0.6.0&k=v"
	ol.Tf(nil, "Server at :%v for http://127.0.0.1:%v/api/v1/gif/as/sls?%v", port, port, query)
	http.ListenAndServe(fmt.Sprintf(":%v", port), nil)
}
