package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"
	"gopkg.in/yaml.v2"
)

var anchorCached = AnchorData2{}
var anchorConfigs = []*AnchorConfig{}
var hardcoreDelay = 0

var listenAddr string

type AnchorData2 struct {
	lock       sync.Mutex
	height     uint64
	blockCount int
}

type AnchorRespBlock struct {
	Miner string `json:"Miner"`
}

type AnchorRespData struct {
	Height int64              `json:"Height"`
	Blocks []*AnchorRespBlock `json:"Blocks"`
}

type AnchorResp struct {
	Result *AnchorRespData `json:"result"`
}

type AnchorResult struct {
	Height uint64 `json:"height"`
	Blocks int    `json:"blocks"`
	URL    string `json:"url"`
}

type AnchorConfig struct {
	URL     string `yaml:"url"`
	Timeout int    `yaml:"timeout"`

	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

func httpCallAnchor(cfg *AnchorConfig) AnchorResult {
	result := AnchorResult{}
	result.URL = cfg.URL

	client := http.Client{
		Timeout: time.Duration(cfg.Timeout) * time.Second,
	}

	url := cfg.URL
	jsonStr := "{ \"jsonrpc\": \"2.0\", \"method\": \"Filecoin.ChainHead\", \"params\": [], \"id\": 1 }"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer([]byte(jsonStr)))
	if err != nil {
		log.Errorf("httpCallAnchor failed:%v, url:%s", err, url)
		return result
	}

	if cfg.User != "" {
		req.SetBasicAuth(cfg.User, cfg.Password)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Errorf("httpCallAnchor failed:%v, url:%s", err, url)
		return result
	}

	defer resp.Body.Close()
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("httpCallAnchor read body failed:%v", err)
		return result
	}

	if resp.StatusCode != 200 {
		log.Errorf("httpCallAnchor req failed, status %d != 200", resp.StatusCode)
		return result
	}

	aresp := &AnchorResp{}
	err = json.Unmarshal(bodyBytes, aresp)
	if err != nil {
		log.Errorf("httpCallAnchor json decode failed:%v. body str:%s", err, string(bodyBytes))
		return result
	}

	if aresp.Result != nil {
		result.Blocks = len(aresp.Result.Blocks)
		result.Height = uint64(aresp.Result.Height)
		log.Infof("httpCallAnchor ok, aresp.Result Height %d, blocks:%d, url:%s", result.Height, result.Blocks, url)
	} else {
		log.Errorf("httpCallAnchor failed, aresp.Result is nil, body str:%s", string(bodyBytes))
	}

	return result
}

func callAnchor() {
	if hardcoreDelay > 0 {
		time.Sleep(time.Duration(hardcoreDelay) * time.Second)
	}

	var wg sync.WaitGroup
	var results = make([]AnchorResult, len(anchorConfigs))
	for i, c := range anchorConfigs {
		wg.Add(1)

		var idx = i
		var c2 = c
		go func() {
			result := httpCallAnchor(c2)
			results[idx] = result
			wg.Done()
		}()
	}

	wg.Wait()

	var diff = false
	sort.Slice(results, func(i, j int) bool {
		if results[i].Height > results[j].Height {
			return true
		}

		if results[i].Height < results[j].Height {
			return false
		}

		if results[i].Blocks > results[j].Blocks {
			diff = true
			return true
		}

		return false
	})

	if diff {
		log.Warnf("callAnchor has diff blocks")
	}

	result := results[0]
	anchorCached.height = result.Height
	anchorCached.blockCount = result.Blocks

	log.Infof("update anchor cache, height:%d, block count:%d, url:%s", result.Height, result.Blocks, result.URL)
}

func anchorBlocksCountByHeight(height uint64) (int, error) {
	anchorCached.lock.Lock()
	defer anchorCached.lock.Unlock()

	if height < anchorCached.height {
		return 0, fmt.Errorf("lotus current anchor height:%d > req %d", anchorCached.height, height)
	}

	if height == anchorCached.height {
		return anchorCached.blockCount, nil
	}

	callAnchor()

	if height == anchorCached.height {
		return anchorCached.blockCount, nil
	}

	return 0, fmt.Errorf("lotus current anchor height:%d != req %d after call to anchor, maybe call failed, check lotus log",
		anchorCached.height, height)
}

type ServerCfg struct {
	ListenAddr    string          `yaml:"listen_addr"`
	HardcoreDelay int             `yaml:"hardcore_delay"`
	AnchorConfigs []*AnchorConfig `yaml:"anchors"`
}

func loadConfig(configFilePath string) {
	data, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		log.Fatal(err)
	}

	serverCfg := ServerCfg{}
	err = yaml.Unmarshal(data, &serverCfg)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	log.Infof("server configs:%+v", serverCfg)
	listenAddr = serverCfg.ListenAddr
	anchorConfigs = serverCfg.AnchorConfigs
	hardcoreDelay = serverCfg.HardcoreDelay
}

type AnchorHeightReply struct {
	Code int    `json:"code"`
	Err  string `json:"error"`

	Height uint64 `json:"height"`
	Blocks int    `json:"blocks"`
}

// Handler
func handlerHead(c echo.Context) error {
	reply := &AnchorHeightReply{}
	reply.Code = 400
	var height uint64

	hstr := c.QueryParam("height")
	if hstr == "" {
		reply.Err = "no 'height' parameter provided"
		return c.JSON(http.StatusOK, reply)
	} else {
		var err error
		height, err = strconv.ParseUint(hstr, 10, 64)
		if err != nil {
			reply.Err = fmt.Sprintf("%v", err)
			return c.JSON(http.StatusOK, reply)
		}
	}

	blk, err := anchorBlocksCountByHeight(height)
	if err != nil {
		reply.Err = fmt.Sprintf("%v", err)
		return c.JSON(http.StatusOK, reply)
	}

	reply.Code = 200
	reply.Height = height
	reply.Blocks = blk
	return c.JSON(http.StatusOK, reply)
}

func main() {
	configFilePath := flag.String("path", "./yz.yaml", "config file path")
	flag.Parse()

	// Echo instance
	e := echo.New()
	if l, ok := e.Logger.(*log.Logger); ok {
		l.SetHeader("${time_rfc3339} ${level}")
	}

	loadConfig(*configFilePath)

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// Routes
	e.GET("/filecoin/head", handlerHead)

	// Start server
	e.Logger.Fatal(e.Start(listenAddr))
}
