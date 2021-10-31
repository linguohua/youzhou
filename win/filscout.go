package win

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/labstack/gommon/log"
)

type FilscoutRespBlock struct {
	Miner string `json:"miner"`
	CID   string `json:"cid"`
}

type FilscoutRespData struct {
	Height int64                `json:"height"`
	Blocks []*FilscoutRespBlock `json:"blocks"`
}

type FilscoutResp struct {
	Code int               `json:"code"`
	Data *FilscoutRespData `json:"data"`
}

func (fr *FilscoutResp) hasCID(cid string) bool {
	if fr.Data != nil {
		for _, b := range fr.Data.Blocks {
			if b.CID == cid {
				return true
			}
		}
	}

	return false
}

func loadTipsetFromFilscout(height uint64) (*FilscoutResp, bool) {
	client := http.Client{
		Timeout: 3 * time.Second,
	}

	url := fmt.Sprintf("https://api.filscout.com/api/v1/tipset/%d", height)
	resp, err := client.Get(url)
	if err != nil {
		log.Printf("loadTipsetFromFilscout failed:%v, url:%s", err, url)
		return nil, false
	}

	defer resp.Body.Close()
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("loadTipsetFromFilscout read body failed:%v, url:%s", err, url)
		return nil, false
	}

	aresp := &FilscoutResp{}
	err = json.Unmarshal(bodyBytes, aresp)
	if err != nil {
		log.Printf("loadTipsetFromFilscout json decode failed:%v, url:%s", err, url)
		return nil, false
	}

	return aresp, true
}
