package win

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type WinDataStatus struct {
	UnhandledReportsCount int `json:"UnhandledReportsCount"`
	OrphansCount          int `json:"OrphansCount"`
	WinCount              int `json:"WinCount"`

	Orphans []*WinReport `json:"Orphans"`
}

// Handler
func handlerStatus(c echo.Context) error {
	s := wdMgr.status()
	return c.JSON(http.StatusOK, &s)
}

func handlerClear(c echo.Context) error {
	wdMgr.clearHistory()

	s := wdMgr.status()
	return c.JSON(http.StatusOK, &s)
}
