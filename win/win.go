package win

import (
	"fmt"
	"sync"
	"time"

	"github.com/labstack/gommon/log"

	"github.com/labstack/echo/v4"
)

const (
	sizeKeep = 1024
)

var (
	wdMgr = &WinDataMgr{
		reports: make([]*WinReport, 0, sizeKeep),
		orphans: make([]*WinReport, 0, sizeKeep),
		wins:    make([]*WinReport, 0, sizeKeep),

		timeOfHistory: time.Now(),
	}
)

type WinDataMgr struct {
	lock    sync.Mutex
	reports []*WinReport
	orphans []*WinReport
	wins    []*WinReport

	rebaseCounter     int
	anchorWaitCounter int

	timeOfHistory time.Time

	timeOfLastOrphan *time.Time
}

func (mgr *WinDataMgr) clearHistory() {
	mgr.lock.Lock()
	defer mgr.lock.Unlock()

	mgr.orphans = mgr.orphans[0:0]
	mgr.wins = mgr.wins[0:0]

	mgr.timeOfHistory = time.Now()
}

func (mgr *WinDataMgr) status(win bool) *WinDataStatus {
	mgr.lock.Lock()
	defer mgr.lock.Unlock()

	s := &WinDataStatus{
		UnhandledReportsCount: len(mgr.reports),
		OrphansCount:          len(mgr.orphans),
		WinCount:              len(mgr.wins),
		Orphans:               make([]*WinReport, len(mgr.orphans)),
		Duration:              time.Since(mgr.timeOfHistory).String(),

		RebaseCounter:     mgr.rebaseCounter,
		AnchorWaitCounter: mgr.anchorWaitCounter,
	}

	for i, o := range mgr.orphans {
		s.Orphans[i] = o
	}

	if win {
		s.Wins = make([]*WinReport, len(mgr.wins))
		for i, o := range mgr.wins {
			s.Wins[i] = o
		}
	}

	if mgr.timeOfLastOrphan != nil {
		s.LastOrphansTime = time.Since(*mgr.timeOfLastOrphan).String()
	}

	return s
}

func (mgr *WinDataMgr) addWinReport(wr *WinReport) {
	mgr.lock.Lock()
	defer mgr.lock.Unlock()

	if wr.NewBase {
		mgr.rebaseCounter++
	}

	if wr.AnchorWait > 0 {
		mgr.anchorWaitCounter = mgr.anchorWaitCounter + wr.AnchorWait
	}

	mgr.reports = append(mgr.reports, wr)
}

func (mgr *WinDataMgr) addOrphanBlock(wr *WinReport) {
	mgr.lock.Lock()
	defer mgr.lock.Unlock()

	mgr.orphans = append(mgr.orphans, wr)
	if len(mgr.orphans) == sizeKeep {
		// trim
		copy(mgr.orphans[0:sizeKeep/2], mgr.orphans[sizeKeep/2:])
		mgr.orphans = mgr.orphans[0 : sizeKeep/2]
	}

	t := wr.Time
	mgr.timeOfLastOrphan = t
}

func (mgr *WinDataMgr) addWinBlock(wr *WinReport) {
	mgr.lock.Lock()
	defer mgr.lock.Unlock()

	mgr.wins = append(mgr.wins, wr)
	if len(mgr.wins) == sizeKeep {
		// trim
		copy(mgr.wins[0:sizeKeep/2], mgr.wins[sizeKeep/2:])
		mgr.wins = mgr.wins[0 : sizeKeep/2]
	}
}

func (mgr *WinDataMgr) takeWinReports(deadline *time.Time) []*WinReport {
	mgr.lock.Lock()
	defer mgr.lock.Unlock()

	var index = 0
	for _, wr := range mgr.reports {
		if deadline.Before(*wr.Time) {
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

	for i := 0; i < len(reports); {
		r := reports[i]
		height := r.Height
		rsp, ok := tipsetGet(height)

		if ok {
			// next report
			i++
			if rsp.hasCID(r.CID) {
				log.Infof("miner win %s, height:%d", r.Miner, r.Height)
				wdMgr.addWinBlock(r)
			} else {
				wdMgr.addOrphanBlock(r)

				rsp2, ok := tipsetGet(height - 1)
				if ok {
					if r.Parents != len(rsp2.Data.Blocks) {
						r.OrphanReason = fmt.Sprintf("parents not match, %d != %d", r.Parents, len(rsp2.Data.Blocks))
					}
				}

				if len(r.OrphanReason) == 0 {
					dur, err := time.ParseDuration(r.Took)
					if err == nil && dur >= (time.Second*25) {
						r.OrphanReason = fmt.Sprintf("timeout, %s", r.Took)
					}
				}

				if len(r.OrphanReason) == 0 {
					r.OrphanReason = "unknown, check miner log"
				}

				log.Infof("miner orphan %s, height:%d, reason:%s", r.Miner, r.Height, r.OrphanReason)

			}
		} else {
			log.Errorf("failed to get tipset, try later, miner:%s, height:%d", r.Miner, r.Height)
			// retry again
			time.Sleep(3 * time.Second)
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
