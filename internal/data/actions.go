package data

import (
	"call-tester/internal/models"
	"fmt"
	"log"
	"os/exec"
	"time"
)

func ExecuteAction(q *QMISession, action models.DataAction) models.DataActionResult {
	result := models.DataActionResult{
    ActionType: action.Type,
		URL: action.URL,
		Host: action.Host,
		StartedAt: time.Now(),
	}

	rxBefore, txBefore := q.GetStatus()

	switch action.Type {
	case "http_get":
		execHTTPGet(q, action, &result)
	case "ping":
		execPing(q, action, &result)
	case "idle":
		dur := action.DurationSec
		if dur <= 0 {
			dur = 10
		}
		time.Sleep(time.Duration(dur) * time.Second)
		result.Success = true
	default:
		result.ErrorMsg = fmt.Sprintf("unknown action: %s", action.Type)
	}

	rxAfter, txAfter := q.GetStatus()
	result.BytesRx = rxAfter - rxBefore
	result.BytesTx = txAfter - txBefore
	result.EndedAt = time.Now()
	result.DurationSec = result.EndedAt.Sub(result.StartedAt).Seconds()

	log.Printf("[%s] action %s: ok=%v rx=%d tx=%d dur=%.2fs", q.Modem, action.Type, result.Success, result.BytesRx, result.BytesTx, result.DurationSec)
	return result
}

func execHTTPGet(q *QMISession, action models.DataAction, r *models.DataActionResult)  {
	if action.URL == "" {
		r.ErrorMsg = "no url"
		return
	}
	tmo := action.TimeoutSec
	if tmo <= 0 {
		tmo = 30
	}
	out, err := exec.Command("curl", "-sS", "--interface", q.NetIface,
       "-o", "/dev/null", "-w", "%{http_code}",
		 "--max-time", fmt.Sprintf("%d", tmo), action.URL).CombinedOutput()
	if err != nil {
		r.ErrorMsg =fmt.Sprintf("curl: %v (%s)", err, string(out))
		return
	}
	var code int
	fmt.Sscanf(string(out), "%d", &code)
	r.HTTPStatus = code 
	r.Success = code >= 200 && code < 400 
	if !r.Success {
		r.ErrorMsg = fmt.Sprintf("HTTP %d", code)
	}
}

func execPing(q *QMISession, action models.DataAction, r *models.DataActionResult) {
	host := action.Host
	if host == "" {
		host = "8.8.8.8"
	}
	count := action.Count
	if count <= 0 {
		count = 5
	}
	out, err := exec.Command("ping", "-I", q.NetIface, "-c", fmt.Sprintf("%d", count), host).CombinedOutput()
	r.PingStatus = string(out)
	r.Success = err == nil
	if !r.Success {
		r.ErrorMsg = "ping failed"
	}
}
