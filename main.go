package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	oh "github.com/ossrs/go-oryx-lib/http"
	ol "github.com/ossrs/go-oryx-lib/logger"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func main() {
	ctx := context.Background()

	var port int
	flag.IntVar(&port, "port", 0, "Listen port.")

	var logfile string
	flag.StringVar(&logfile, "log", "", "Log file path. Default: stdout")

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

	var f *os.File
	if logfile == "" {
		f = os.Stdout
	} else {
		if lf, err := os.OpenFile(logfile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666); err != nil {
			ol.Ef(ctx, "Open %v err %v", logfile, err)
			os.Exit(-1)
		} else {
			defer lf.Close()
			f = lf
		}
	}

	oh.Server = "go-oryx"

	ol.Tf(ctx, "Handle /")
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/gif") {
			http.NotFound(w, r)
			return
		}

		rip := GetOriginalClientIP(r)
		ua := r.Header.Get("User-Agent")
		referer := r.Header.Get("Referer")
		rawURL := r.URL.RawQuery

		u, err := url.Parse(rawURL)
		if err != nil {
			oh.WriteError(ctx, w, r, err)
			return
		}

		var logstore string
		if logstores := strings.SplitN(u.Path, "/", 4); len(logstores) > 3 {
			logstore = logstores[2]
		}

		var project string
		if projects := strings.SplitN(u.Host, ".", 2); len(projects) > 1 {
			project = projects[0]
		}

		q := u.Query()
		q.Del("APIVersion")
		q.Set("ip", rip)
		q.Set("referer", referer)
		q.Set("ua", ua)
		q.Set("project", project)
		q.Set("logstore", logstore)
		ol.Tf(ctx, "Turn url=%v to %v", rawURL, q)

		qq := make(map[string]string)
		for k, _ := range q {
			qq[k] = q.Get(k)
		}
		bb, err := json.Marshal(qq)
		if err != nil {
			oh.WriteError(ctx, w, r, err)
			return
		}
		io.WriteString(f, string(bb))

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
	help := "https://help.aliyun.com/document_detail/31752.html"
	ol.Tf(ctx, "Server at :%v for http://127.0.0.1:%v/gif/v1/sls?%v at %v", port, port, query, help)
	http.ListenAndServe(fmt.Sprintf(":%v", port), nil)
}

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
