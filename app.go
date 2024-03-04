package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/signal"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

var (
	cIpM = make(map[string]map[string]bool, 10)
	mu   = &sync.Mutex{}
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	help := flag.Bool("h", false, "Print this help")
	pxy := flag.String("x", "", "Proxy Host:Port")
	user := flag.String("u", "", "ProxyUser: xxx-{country}-xxx-{session}")
	pwd := flag.String("p", "", "ProxyPwd")
	sessions := flag.Int("ses", 5, "Sessions per country")
	sesBefore := flag.Int("b", 0, "SessionID begin with")
	countries := flag.String("cs", "BR,IN,ID,MX", "Countries comma seperated")
	testUrl := flag.String("t", "https://lumtest.com/myip.json", "TestIPUrl")
	outFile := flag.String("o", "log/pxy-ip.csv", "OutputFile")
	flag.Parse()
	if *help {
		flag.PrintDefaults()
		return
	}
	if *pxy == "" {
		log.Fatal("Proxy required")
	}
	if *user == "" {
		log.Fatal("ProxyUser required")
	}
	if *pwd == "" {
		log.Fatal("ProxyPwd required")
	}
	if *countries == "" {
		log.Fatal("countries required")
	}

	if err := os.MkdirAll(path.Dir(*outFile), 0755); err != nil {
		log.Fatal("CreateDirErr:", err)
	}
	f, err := os.Create(*outFile)
	if err != nil {
		log.Fatal("CreateFileErr:", err)
	}
	defer f.Close()

	go func() {
		quit := make(chan os.Signal)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		log.Println("Server exiting\n---------------\n=\n=")
		os.Exit(130)
	}()

	_, _ = f.WriteString("Country,Session,Code,IP,IP-Country\n")
	testUri, _ := url.Parse(*testUrl)
	par := 500
	cd := &sync.WaitGroup{}
	cd.Add(par)
	ch := make(chan *Executor, par)
	processed := new(atomic.Int32)
	for i := 0; i < par; i++ {
		go work(ch, cd, processed, f)
	}
	cs := strings.Split(*countries, ",")
	total := int32(len(cs) * *sessions)
	go func() {
		for processed.Load() < total {
			i := processed.Load()
			log.Printf("Processed: %d / %d, %.1f%%\n", i, total, float32(i)*100/float32(total))
			time.Sleep(time.Second * 2)
		}
	}()

	for _, c := range cs {
		for i := 0; i < *sessions; i++ {
			ch <- &Executor{
				Country: c,
				Session: i + *sesBefore,
				Proxy:   getProxy(pxy, user, pwd, c, i+*sesBefore),
				Uri:     testUri,
			}
		}
	}
	close(ch)
	cd.Wait()

	log.Println("Country, IP-Count")
	for c, m := range cIpM {
		log.Printf("%s, %d\n", c, len(m))
	}
	log.Print("Finished\n---\n--\n-\n")
}

func work(ch chan *Executor, cd *sync.WaitGroup, processed *atomic.Int32, f io.Writer) {
	defer cd.Done()
	for exe := range ch {
		exe.Execute()
		r := exe.Result
		v := &ipRv{}
		_ = json.Unmarshal(r.Body, v)
		_, _ = fmt.Fprintf(f, "%s,%d,%d,%s,%s\n", exe.Country, exe.Session, r.Code, v.Ip, v.Country)
		processed.Add(1)

		if v.Ip != "" && v.Country == exe.Country {
			mu.Lock()
			m, ok := cIpM[exe.Country]
			if !ok {
				m = make(map[string]bool, 10000)
				cIpM[exe.Country] = m
			}
			m[v.Ip] = true
			mu.Unlock()
		}
	}
}

type ipRv struct {
	Ip      string `json:"ip"`
	Country string `json:"country"`
}
