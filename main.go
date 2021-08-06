package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	_ "github.com/aliyun/alibaba-cloud-sdk-go/sdk"
	_ "github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
	_ "github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	_ "github.com/aliyun/alibaba-cloud-sdk-go/services/sts"
	"github.com/aliyun/aliyun-log-go-sdk"
	"github.com/golang/protobuf/proto"
	oe "github.com/ossrs/go-oryx-lib/errors"
	oh "github.com/ossrs/go-oryx-lib/http"
	ol "github.com/ossrs/go-oryx-lib/logger"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func main() {
	ctx := context.Background()
	clients = make(map[string]*sls.Client)

	var conf string
	flag.StringVar(&conf, "c", "", "The config file path")

	flag.Usage = func() {
		fmt.Println(fmt.Sprintf("HTTP GIF as SLS writer"))
		flag.PrintDefaults()
		fmt.Println(fmt.Sprintf("For example:"))
		fmt.Println(fmt.Sprintf("    %v -c main.conf", os.Args[0]))
	}

	flag.Parse()

	if conf == "" {
		flag.Usage()
		os.Exit(-1)
	}

	co := Config{}
	if err := func() error {
		f, err := os.Open(conf)
		if err != nil {
			return err
		}
		defer f.Close()

		b, err := ioutil.ReadAll(f)
		if err != nil {
			return err
		}

		if err := json.Unmarshal(b, &co); err != nil {
			return err
		}

		return nil
	}(); err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
	ol.Tf(ctx, "Run with conf=%v, port=%v, file=(%v,%v), aliyun(%v,%v,%v,%v,%v,%v)", conf,
		co.Port, co.LogFile.Enabled, co.LogFile.Tank, co.LogAliyunAK.Enabled, co.LogAliyunAK.ID,
		co.LogAliyunAK.Topic, co.LogAliyunAK.Project, co.LogAliyunAK.LogStore, co.LogAliyunAK.Endpoint)

	var f *os.File
	if co.LogFile.Enabled {
		if co.LogFile.Tank == "" || co.LogFile.Tank == "stdout" {
			f = os.Stdout
		} else {
			if lf, err := os.OpenFile(co.LogFile.Tank, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666); err != nil {
				ol.Ef(ctx, "Open %v err %v", co.LogFile.Tank, err)
				os.Exit(-1)
			} else {
				defer lf.Close()
				f = lf
			}
		}
	}

	oh.Server = "go-oryx"

	ol.Tf(ctx, "Handle /")
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		cp := co.Parse(q)
		logForApp, keepReferer, keepUA, keepOReferer, keepOUA, keepFWD := parseLogForApp(q)

		if !strings.HasPrefix(r.URL.Path, "/gif") && !logForApp {
			ol.Wf(ctx, "Ignore %v of %v", r.URL.Path, r.URL.String())
			http.NotFound(w, r)
			return
		}

		rip := GetOriginalClientIP(r)
		ua := r.Header.Get("User-Agent")
		referer := r.Header.Get("Referer")
		rawURL := r.URL.RawQuery

		if logForApp {
			q.Set("rip", rip)
			if keepOReferer {
				q.Set("oreferer", referer)
			}
			if keepOUA {
				q.Set("oua", ua)
			}
			if keepReferer {
				q.Set("referer", reparseReferer(referer))
			}
			if keepUA {
				q.Set("ua", reparseUserAgent(ua))
			}
			if keepFWD {
				q.Set("fwd", q.Get("X-Forwarded-For"))
			}
		} else {
			q.Set("oreferer", referer)
			q.Set("oua", ua)
			q.Set("__tag__:__client_ip__", rip)
			q.Set("__referer__", reparseReferer(referer))
			q.Set("__userAgent__", reparseUserAgent(ua))
		}

		qq := make(map[string]string)
		for k, _ := range q {
			if q.Get(k) != "" {
				qq[k] = q.Get(k)
			}
		}
		if err := writeSlsLog(ctx, &cp, qq); err != nil {
			oh.WriteError(ctx, w, r, err)
			return
		}

		bb, err := json.Marshal(qq)
		if err != nil {
			oh.WriteError(ctx, w, r, err)
			return
		}
		if f != nil {
			if _, err := io.WriteString(f, string(bb)+"\n"); err != nil {
				oh.WriteError(ctx, w, r, err)
				return
			}
		}
		ol.Tf(ctx, "Stat as %v from url=%v config=%v", string(bb), rawURL, cp)

		if logForApp {
			w.Write(nil)
			return
		}

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
	help := "https://help.aliyun.com/document_detail/31752.html"
	ol.Tf(ctx, "Server at :%v for http://127.0.0.1:%v/gif/v1/sls.gif?site=ossrs.net&path=/release/docker @see %v", co.Port, co.Port, help)
	ol.Tf(ctx, "->Note that ?_sys_project=xxx to overwrite the SLS project")
	ol.Tf(ctx, "->Note that ?_sys_logstore=xxx to overwrite the SLS logstore")
	ol.Tf(ctx, "->Note that ?_sys_endpoint=xxx to overwrite the SLS endpoint")
	ol.Tf(ctx, "->Note that ?_sys_logfmt=app to write raw data, without web fields")
	ol.Tf(ctx, "->Note that ?_sys_keep_referer=true to keep the referer")
	ol.Tf(ctx, "->Note that ?_sys_keep_ua=true to keep the ua")
	ol.Tf(ctx, "->Note that ?_sys_keep_oreferer=true to keep the original referer")
	ol.Tf(ctx, "->Note that ?_sys_keep_oua=true to keep the original ua")
	ol.Tf(ctx, "->Note that ?_sys_keep_fwd=true to keep the original X-Forwarded-For")
	http.ListenAndServe(fmt.Sprintf(":%v", co.Port), nil)
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

func slsKVEncode(ctx context.Context, kvs map[string]string) ([]*sls.LogContent, error) {
	if kvs == nil {
		return nil, oe.New("nil")
	}

	var contents []*sls.LogContent
	for key, value := range kvs {
		contents = append(contents, &sls.LogContent{
			Key: proto.String(key), Value: proto.String(value),
		})
	}

	ol.If(ctx, "Encode JSON %v as KV %v", kvs, contents)

	return contents, nil
}

var clients map[string]*sls.Client

func buildSlsClient(co *Config) *sls.Client {
	if !co.LogAliyunAK.Enabled {
		return nil
	}

	if v, ok := clients[co.LogAliyunAK.Endpoint]; ok {
		return v
	}

	client := &sls.Client{}
	client.Endpoint = co.LogAliyunAK.Endpoint
	client.AccessKeyID = co.LogAliyunAK.ID
	client.AccessKeySecret = co.LogAliyunAK.Secret
	clients[co.LogAliyunAK.Endpoint] = client
	return client
}

type Config struct {
	Port    int `json:"port"`
	LogFile struct {
		Enabled bool   `json:"enabled"`
		Tank    string `json:"tank"`
	} `json:"log_file"`
	LogAliyunAK struct {
		Enabled  bool   `json:"enabled"`
		ID       string `json:"id"`
		Secret   string `json:"secret"`
		Topic    string `json:"topic"`
		Project  string `json:"project"`
		LogStore string `json:"logstore"`
		Endpoint string `json:"endpoint"`
	} `json:"log_aliyun_ak"`
}

func (co Config) String() string {
	return fmt.Sprintf("project=%v, logstore=%v, endpoint=%v", co.LogAliyunAK.Project, co.LogAliyunAK.LogStore, co.LogAliyunAK.Endpoint)
}

func (co Config) Parse(q url.Values) Config {
	cp := co
	for k, v := range map[string]*string{
		"_sys_project":  &cp.LogAliyunAK.Project,
		"_sys_logstore": &cp.LogAliyunAK.LogStore,
		"_sys_endpoint": &cp.LogAliyunAK.Endpoint,
	} {
		if qv := q.Get(k); qv != "" {
			q.Del(k)
			*v = qv
		}
	}

	return cp
}

func parseLogForApp(q url.Values) (logForApp, keepReferer, keepUA, keepOReferer, keepOUA, keepFWD bool) {
	for k, v := range map[string]*bool{
		"_sys_logfmt_app":    &logForApp,
		"_sys_keep_referer":  &keepReferer,
		"_sys_keep_ua":       &keepUA,
		"_sys_keep_oreferer": &keepOReferer,
		"_sys_keep_oua":      &keepOUA,
		"_sys_keep_fwd":      &keepFWD,
	} {
		if qv := q.Get(k); qv != "" {
			q.Del(k)
			if qv == "true" {
				*v = true
			}
		}
	}
	return
}

func writeSlsLog(ctx context.Context, co *Config, qq map[string]string) error {
	client := buildSlsClient(co)
	if client == nil {
		return nil
	}

	contents, err := slsKVEncode(ctx, qq)
	if err != nil {
		return err
	}

	logGroup := &sls.LogGroup{
		Topic: proto.String(co.LogAliyunAK.Topic),
		Logs: []*sls.Log{
			&sls.Log{
				Time:     proto.Uint32(uint32(time.Now().Unix())),
				Contents: contents,
			},
		},
	}
	err = client.PutLogs(co.LogAliyunAK.Project, co.LogAliyunAK.LogStore, logGroup)
	if err != nil {
		return err
	}

	return nil
}

func reparseReferer(referer string) string {
	if referer != "" {
		if u, err := url.Parse(referer); err == nil {
			return u.Host
		}
	}
	return referer
}

func reparseUserAgent(ua string) string {
	if strings.Contains(ua, "Mac OS X") && strings.Contains(ua, "Macintosh") {
		// Mozilla/5.0 (Macintosh; Intel Mac OS X 10_14_5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/78.0.3904.108 Safari/537.36
		return "macOS"
	} else if strings.Contains(ua, "Windows") {
		// Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/77.0.3865.90 Safari/537.36
		// Mozilla/5.0 (Windows NT 6.1; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/78.0.3904.108 Safari/537.36
		return "windows"
	} else if strings.Contains(ua, "Android") {
		// Mozilla/5.0 (Linux; U; Android 8.1.0; zh-CN; EML-AL00 Build/HUAWEIEML-AL00) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/57.0.2987.108 baidu.sogo.uc.UCBrowser/11.9.4.974 UWS/2.13.1.48 Mobile Safari/537.36 AliApp(DingTalk/4.5.11) com.alibaba.android.rimet/10487439 Channel/227200 language/zh-CN
		return "android"
	} else if strings.Contains(ua, "iPhone") {
		// Mozilla/5.0 (iPhone; CPU iPhone OS 7_0 like Mac OS X) AppleWebKit/537.51.1 (KHTML, like Gecko) Version/7.0 Mobile/11A465 Safari/9537.53 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)
		return "ios"
	} else if strings.Contains(ua, "Linux") {
		// Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/78.0.3904.108 Safari/537.36
		// Mozilla/5.0 (X11; Linux x86_64; rv:68.0) Gecko/20100101 Firefox/68.0
		return "linux"
	} else if strings.HasPrefix(ua, "github-camo") || strings.HasPrefix(ua, "search-http-client") {
		// github-camo (876de43e)
		// search-http-client
		return "agent"
	} else if strings.Contains(ua, "spider") {
		// Mozilla/5.0 (compatible; Baiduspider-render/2.0; +http://www.baidu.com/search/spider.html)
		return "spider"
	} else if strings.Contains(ua, "curl") {
		// curl/7.54.0
		return "curl"
	} else if strings.Contains(ua, "Go-http") {
		// Go-http-client/1.1
		return "go"
	} else if len(ua) > 8 {
		return ua[:8]
	}
	return ua
}
