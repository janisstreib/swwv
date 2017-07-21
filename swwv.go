package main

import (
	"fmt"
	"golang.org/x/net/html"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
)

func main() {
	server := http.Server{
		Addr:    ":8000",
		Handler: &myHandler{},
	}
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
            fmt.Println(ref)
			refUrl, err := url.Parse(ref)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			fmt.Print("Raw path ")
			fmt.Println(refUrl.Path)
			refUrl, err = url.Parse(refUrl.Path[1:])
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			fmt.Print("Patching url by ref to ")
			reqUrl = normalizeUrl(refUrl, "/"+reqUrl)[1:]
			fmt.Println(reqUrl)
		}
	}
	req, err := http.NewRequest(r.Method, reqUrl, nil)
	if err != nil {
		// FIXME
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	//req.Header = r.Header
	resp, err := client.Do(req)
	if err != nil {
		// FIXME
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	ct, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	switch ct {
	case "text/html":
		fmt.Println("html")
		root, err := html.Parse(resp.Body)
		if err != nil {
			fmt.Println(err)
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
