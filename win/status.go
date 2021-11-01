package win

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type WinDataStatus struct {
	UnhandledReportsCount int `json:"UnhandledReportsCount"`
	OrphansCount          int `json:"OrphansCount"`
	WinCount              int `json:"WinCount"`

	Wins    []*WinReport `json:"Wins"`
	Orphans []*WinReport `json:"Orphans"`
}

// Handler
func handlerStatus(c echo.Context) error {
	win := c.QueryParam("win") == "1"

	s := wdMgr.status(win)

	return c.JSONPretty(http.StatusOK, &s, "    ")
}

func handlerClear(c echo.Context) error {
	wdMgr.clearHistory()

	s := wdMgr.status(false)
	return c.JSON(http.StatusOK, &s)
}
