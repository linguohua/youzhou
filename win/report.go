package win

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

type WinReport struct {
	Miner  string `json:"miner"`
	CID    string `json:"cid"`
	Height uint64 `json:"height"`
	Took   string `json:"took"`

	time time.Time
}

// Handler
func handlerReport(c echo.Context) error {
	wr := new(WinReport)
	if err := c.Bind(wr); err != nil {
		return c.String(400, err.Error())
	}

	if wr.CID == "" || wr.Miner == "" || wr.Height == 0 {
		return c.String(400, "invalid report")
	}

	wr.time = time.Now()
	wdMgr.addWinReport(wr)

	return c.JSON(http.StatusOK, wr)
}
