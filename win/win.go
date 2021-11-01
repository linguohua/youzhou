package win

import (
	"sync"
	"time"

	"github.com/labstack/gommon/log"

	"github.com/labstack/echo/v4"
)

var (
	wdMgr = &WinDataMgr{
		reports: make([]*WinReport, 0, 1024),
		orphans: make([]*WinReport, 0, 1024),
		wins:    make([]*WinReport, 0, 1024),
	}
)

type WinDataMgr struct {
	lock    sync.Mutex
	reports []*WinReport
	orphans []*WinReport
	wins    []*WinReport
}

func (mgr *WinDataMgr) clearHistory() {
	mgr.lock.Lock()
	defer mgr.lock.Unlock()

	mgr.orphans = mgr.orphans[0:0]
	mgr.wins = mgr.wins[0:0]
}

func (mgr *WinDataMgr) status() *WinDataStatus {
	mgr.lock.Lock()
	defer mgr.lock.Unlock()

	s := &WinDataStatus{
		UnhandledReportsCount: len(mgr.reports),
		OrphansCount:          len(mgr.orphans),
		WinCount:              len(mgr.wins),
		Orphans:               make([]*WinReport, len(mgr.orphans)),
	}

	for i, o := range mgr.orphans {
		s.Orphans[i] = o
	}

	return s
}

func (mgr *WinDataMgr) addWinReport(wr *WinReport) {
	mgr.lock.Lock()
	defer mgr.lock.Unlock()

	mgr.reports = append(mgr.reports, wr)
}

func (mgr *WinDataMgr) addOrphanBlock(wr *WinReport) {
	mgr.lock.Lock()
	defer mgr.lock.Unlock()

	mgr.orphans = append(mgr.orphans, wr)
}

func (mgr *WinDataMgr) addWinBlock(wr *WinReport) {
	mgr.lock.Lock()
	defer mgr.lock.Unlock()

	mgr.wins = append(mgr.wins, wr)
}

func (mgr *WinDataMgr) takeWinReports(deadline *time.Time) []*WinReport {
	mgr.lock.Lock()
	defer mgr.lock.Unlock()

	var index = 0
	for _, wr := range mgr.reports {
		if deadline.Before(wr.time) {
			break
		}
		index++
	}

	if index > 0 {
		l := len(mgr.reports)
		result := make([]*WinReport, index)
		copy(result, mgr.reports[0:index])

		if l > index {
			copy(mgr.reports[0:l-index], mgr.reports[index:l])
		}
		mgr.reports = mgr.reports[0 : l-index]

		return result
	}

	return nil
}

func processWinReports() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovering from panic in processWinReports error is: %v \n", r)
		}
	}()

	now := time.Now()
	dealine := now.Add(-5 * time.Minute)
	reports := wdMgr.takeWinReports(&dealine)

	if len(reports) < 1 {
		return
	}

	cached := make(map[uint64]*FilscoutResp)
	tipsetGet := func(h uint64) (*FilscoutResp, bool) {
		rsp, ok := cached[h]
		if !ok {
			rsp, ok = loadTipsetFromFilscout(h)
			if ok {
				cached[h] = rsp
			}
		}

		return rsp, ok
	}

	for _, r := range reports {
		height := r.Height
		rsp, ok := tipsetGet(height)

		if ok {
			if rsp.hasCID(r.CID) {
				log.Infof("miner win %s, height:%d", r.Miner, r.Height)
				wdMgr.addWinBlock(r)
			} else {
				rsp2, ok := tipsetGet(height - 1)
				if ok {
					if r.Parents != len(rsp2.Data.Blocks) {
						r.OrphanReason = "parents not match"
					}
				}

				if len(r.OrphanReason) == 0 {
					dur, err := time.ParseDuration(r.Took)
					if err == nil && dur >= (time.Second*25) {
						r.OrphanReason = "timeout"
					}
					wdMgr.addOrphanBlock(r)
				}

				if len(r.OrphanReason) == 0 {
					r.OrphanReason = "unknown"
				}

				log.Infof("miner orphan %s, height:%d, reason:%s", r.Miner, r.Height, r.OrphanReason)
			}
		} else {
			log.Errorf("failed to get tipset, drop report, miner:%s, height:%d", r.Miner, r.Height)
		}
	}
}

func winDaemon() {
	for {
		time.Sleep(time.Minute)
		processWinReports()
	}
}

func Register(e *echo.Echo) {
	e.POST("/filecoin/win/report", handlerReport)
	e.GET("/filecoin/win/status", handlerStatus)
	e.POST("/filecoin/win/clear", handlerClear)

	go winDaemon()
}
