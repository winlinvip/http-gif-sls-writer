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
			ol.Wf(ctx, "Ignore %v of %v", r.URL.Path, r.URL.String())
			http.NotFound(w, r)
			return
		}

		rip := GetOriginalClientIP(r)
		ua := r.Header.Get("User-Agent")
		referer := r.Header.Get("Referer")
		rawURL := r.URL.RawQuery

		q := r.URL.Query()
		if strings.Contains(rawURL, "://") {
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

			q = u.Query()
			q.Del("APIVersion")
			q.Set("project", project)
			q.Set("logstore", logstore)
		}

		q.Set("__tag__:__client_ip__", rip)

		q.Set("oreferer", referer)
		if referer != "" {
			if u, err := url.Parse(referer); err == nil {
				referer = u.Host
			}
			q.Set("__referer__", referer)
		}

		q.Set("oua", ua)
		if strings.Contains(ua, "Mac OS X") && strings.Contains(ua, "Macintosh") {
			// Mozilla/5.0 (Macintosh; Intel Mac OS X 10_14_5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/78.0.3904.108 Safari/537.36
			ua = "macOS"
		} else if strings.Contains(ua, "Windows") {
			// Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/77.0.3865.90 Safari/537.36
			// Mozilla/5.0 (Windows NT 6.1; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/78.0.3904.108 Safari/537.36
			ua = "windows"
		} else if strings.Contains(ua, "Android") {
			// Mozilla/5.0 (Linux; U; Android 8.1.0; zh-CN; EML-AL00 Build/HUAWEIEML-AL00) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/57.0.2987.108 baidu.sogo.uc.UCBrowser/11.9.4.974 UWS/2.13.1.48 Mobile Safari/537.36 AliApp(DingTalk/4.5.11) com.alibaba.android.rimet/10487439 Channel/227200 language/zh-CN
			ua = "android"
		} else if strings.Contains(ua, "iPhone") {
			// Mozilla/5.0 (iPhone; CPU iPhone OS 7_0 like Mac OS X) AppleWebKit/537.51.1 (KHTML, like Gecko) Version/7.0 Mobile/11A465 Safari/9537.53 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)
			ua = "ios"
		} else if strings.Contains(ua, "Linux") {
			// Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/78.0.3904.108 Safari/537.36
			// Mozilla/5.0 (X11; Linux x86_64; rv:68.0) Gecko/20100101 Firefox/68.0
			ua = "linux"
		} else if strings.HasPrefix(ua, "github-camo") || strings.HasPrefix(ua, "search-http-client") {
			// github-camo (876de43e)
			// search-http-client
			ua = "agent"
		} else if strings.Contains(ua, "spider") {
			// Mozilla/5.0 (compatible; Baiduspider-render/2.0; +http://www.baidu.com/search/spider.html)
			ua = "spider"
		} else if strings.Contains(ua, "curl") {
			// curl/7.54.0
			ua = "curl"
		}
		if ua != "" {
			q.Set("__userAgent__", ua)
		}

		qq := make(map[string]string)
		for k, _ := range q {
			qq[k] = q.Get(k)
		}
		bb, err := json.Marshal(qq)
		if err != nil {
			oh.WriteError(ctx, w, r, err)
			return
		}
		if _, err := io.WriteString(f, string(bb)+"\n"); err != nil {
			oh.WriteError(ctx, w, r, err)
			return
		}
		ol.Tf(ctx, "Stat as %v from url=%v", string(bb), rawURL)

		h := w.Header()
		h.Set("Content-Type", "image/gif")
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
