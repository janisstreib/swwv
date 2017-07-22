package main

import (
	"flag"
	"github.com/Sirupsen/logrus"
	"golang.org/x/net/html"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
)

var mainLog = logrus.WithField("module", "main")

type LogLevelFlag struct {
	// flag.Value
	lvl logrus.Level
}

func (f LogLevelFlag) String() string {
	return f.lvl.String()
}
func (f LogLevelFlag) Set(val string) error {
	l, err := logrus.ParseLevel(val)
	if err != nil {
		f.lvl = l
	}
	return err
}

func main() {
	var (
		logLevel     LogLevelFlag = LogLevelFlag{logrus.DebugLevel}
		listenString string
	)

	flag.Var(&logLevel, "log.level", "possible values: debug, info, warning, error, fatal, panic")
	flag.StringVar(&listenString, "listen", ":8080", "net.Listen() string, e.g. addr:port")

	flag.Parse()
	logrus.SetLevel(logLevel.lvl)

	server := http.Server{
		Addr:    listenString,
		Handler: &myHandler{},
	}
	mainLog.Info("Startup")
	server.ListenAndServe()
}

type myHandler struct{}

func buildRootURLString(url *url.URL) string {
	return url.Scheme + "://" + url.Host
}

func normalizeUrl(targetHost *url.URL, url string) string {
	return "/" + buildRootURLString(targetHost) + url
}

func (*myHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reqUrl := r.URL.String()
	reqUrl = reqUrl[1:]
	client := &http.Client{}
	if !strings.HasPrefix(reqUrl, "http") {
		if ref := r.Header.Get("Referer"); ref != "" {
			// FIXME: Security: diable cookie caching
			mainLog.Debug("Referer: ", ref)
			refUrl, err := url.Parse(ref)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			mainLog.Debug("Referer URL: ", refUrl.Path)
			refUrl, err = url.Parse(refUrl.Path[1:])
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			reqUrl = normalizeUrl(refUrl, "/"+reqUrl)[1:]
			mainLog.Debug("Patching url by ref to ", reqUrl)
		}
	}
	req, err := http.NewRequest(r.Method, reqUrl, nil)
	if err != nil {
		// FIXME
		mainLog.Error("Error creating http-client request object! ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	//req.Header = r.Header
	resp, err := client.Do(req)
	if err != nil {
		// FIXME
		mainLog.Error("Error while doing http-client request! ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	ct, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		mainLog.Error("Error parsing mime-type! ", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	switch ct {
	case "text/html":
		root, err := html.Parse(resp.Body)
		if err != nil {
			mainLog.Error("Error pasring the html! ", err)
			return
		}
		var f func(*html.Node)
		f = func(node *html.Node) {
			if node.Type == html.ElementNode {
				if node.Data == "form" || node.Data == "script" || node.Data == "link" || node.Data == "a" || node.Data == "img" {
					for i, attr := range node.Attr {
						if attr.Key == "href" || attr.Key == "src" || attr.Key == "action" {
							if strings.HasPrefix(attr.Val, "//") {
								node.Attr[i].Val = "/" + req.URL.Scheme + ":" + attr.Val
							} else if strings.HasPrefix(attr.Val, "http://") || strings.HasPrefix(attr.Val, "https://") {
								node.Attr[i].Val = "/" + attr.Val
							} else if strings.HasPrefix(attr.Val, "/") {
								node.Attr[i].Val = normalizeUrl(req.URL, attr.Val)
							}
						}
					}
				}
			}
			for c := node.FirstChild; c != nil; c = c.NextSibling {
				f(c)
			}
		}
		f(root)
		html.Render(w, root)
	default:
		w.Header().Set("Content-Type", req.Header.Get("Content-Type"))
		io.Copy(w, resp.Body)
	}
}
