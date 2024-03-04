package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var transport = &http.Transport{
	IdleConnTimeout:       10 * time.Second,
	ResponseHeaderTimeout: 30 * time.Second,
	DisableKeepAlives:     true,
	Proxy: func(req *http.Request) (*url.URL, error) {
		d := req.Context().Value("executor").(*Executor)
		return d.Proxy, nil
	},
	OnProxyConnectResponse: func(ctx context.Context, pxy *url.URL, connReq *http.Request, connRes *http.Response) error {
		if connRes.StatusCode == http.StatusOK {
			return nil
		}
		var buf bytes.Buffer
		buf.Grow(1024)
		_, _ = fmt.Fprintf(&buf, "%s %s\n", connRes.Proto, connRes.Status)
		for k, vs := range connRes.Header {
			for _, v := range vs {
				_, _ = fmt.Fprintf(&buf, "%s: %s\n", k, v)
			}
		}
		log.Printf("%v, hs: \n%s\n", pxy, buf.String())
		return nil
	},
}

func getProxy(gw, userPtr, pwdPtr *string, country string, session int) *url.URL {
	uri := &url.URL{Scheme: "http", Host: *gw}
	user := strings.ReplaceAll(strings.ReplaceAll(*userPtr, "{country}", country), "{session}", strconv.Itoa(session))
	uri.User = url.UserPassword(user, *pwdPtr)
	return uri
}

type Executor struct {
	Country string
	Session int
	Proxy   *url.URL
	Uri     *url.URL
	Result  *DetectResult
}

type DetectResult struct {
	Code int    `json:"code"`
	Msg  string `json:"msg,omitempty"`
	Body []byte `json:"metrics,omitempty"`
}

func (d *DetectResult) String() string {
	return fmt.Sprintf("Code: %d, Msg: %s, Body: %s", d.Code, d.Msg, d.Body)
}

func (d *Executor) Execute() {
	d.Result = &DetectResult{}
	ctx := context.WithValue(context.Background(), "executor", d)
	d.doExecute(ctx)
}

func (d *Executor) doExecute(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.Uri.String(), nil)
	if err != nil {
		log.Println("NewReqErr:", err)
		return
	}
	res, err := transport.RoundTrip(req)
	if err != nil {
		d.Result.Code = 500
		d.Result.Msg = err.Error()
		log.Println("DoReqErr:", err)
		return
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		d.Result.Code = 400
		d.Result.Msg = err.Error()
		log.Println("ReadBodyErr:", err)
		return
	}
	d.Result.Code = res.StatusCode
	d.Result.Msg = res.Status
	d.Result.Body = body
}
