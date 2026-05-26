package engine

import (
	"call-tester/internal/data"
	"call-tester/internal/metrics"
	"call-tester/internal/modem"
	"call-tester/internal/models"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Registry хранит модемы и конфиги
type Registry struct {
	ctrls    map[string]*modem.Controller
	configs  map[string]models.ModemConfig
	tableIDs map[string]int
}

func NewRegistry(config *models.SystemConfig) (*Registry, error) {
	r := &Registry{
		ctrls:    make(map[string]*modem.Controller),
		configs:  make(map[string]models.ModemConfig),
		tableIDs: make(map[string]int),
	}
	for i, cfg := range config.Modems {
		log.Printf("connecting '%s' (%s, %s)", cfg.Name, cfg.Model, cfg.Operator)
		ctrl, err := modem.New(cfg.ATPort, cfg.BaudRate, cfg.Name, cfg.PhoneNumber, cfg.Model, cfg.Operator)
		if err != nil {
			log.Printf("  ERROR: %v (skipping)", err)
			continue
		}
		ctrl.QMIDevice = cfg.QMIDevice
		ctrl.NetIface = cfg.NetIface
		ctrl.APN = cfg.APN
		ctrl.APNUser = cfg.APNUser
		ctrl.APNPass = cfg.APNPass

		if err := ctrl.Init(); err != nil {
			log.Printf("  ERROR init: %v (skipping)", err)
			ctrl.Close()
			continue
		}
		rssi, _, err := ctrl.GetSignalQuality()
		if err == nil {
			metrics.ModemSignalRSSI.WithLabelValues(cfg.Name).Set(float64(rssi))
			if rssi != 99 {
				metrics.ModemSignalDBm.WithLabelValues(cfg.Name).Set(float64(-113 + rssi*2))
			}
			metrics.ModemRegistered.WithLabelValues(cfg.Name, cfg.Operator).Set(1)
			log.Printf("  [%s] signal: %s", cfg.Name, models.SignalDBm(rssi))
		} else {
			metrics.ModemRegistered.WithLabelValues(cfg.Name, cfg.Operator).Set(0)
		}
		r.ctrls[cfg.Name] = ctrl
		r.configs[cfg.Name] = cfg
		r.tableIDs[cfg.Name] = 100 + i
	}
	return r, nil
}

func (r *Registry) Close() {
	for _, c := range r.ctrls {
		c.Close()
	}
}

// ---- Scenario execution ----

type Report struct {
	mu   sync.Mutex
	data models.ScenarioReport
}

func (rp *Report) AddCall(rec models.CallRecord)  { rp.mu.Lock(); rp.data.Calls = append(rp.data.Calls, rec); rp.mu.Unlock() }
func (rp *Report) AddSMS(rec models.SmsRecord)     { rp.mu.Lock(); rp.data.SMS = append(rp.data.SMS, rec); rp.mu.Unlock() }
func (rp *Report) AddData(rec models.DataRecord)    { rp.mu.Lock(); rp.data.Data = append(rp.data.Data, rec); rp.mu.Unlock() }

func Execute(reg *Registry, scenario *models.Scenario) models.ScenarioReport {
	log.Printf("=== START: '%s' (%d steps) ===", scenario.Name, len(scenario.Steps))
	report := &Report{data: models.ScenarioReport{
		ScenarioName: scenario.Name,
		ExecutedAt:   time.Now(),
	}}
	for i, step := range scenario.Steps {
		log.Printf("--- step %d/%d: %s ---", i+1, len(scenario.Steps), step.Action)
		executeStep(reg, report, step, scenario.Name, i)
	}
	computeSummary(&report.data)
	log.Printf("=== DONE ===")
	return report.data
}

func executeStep(reg *Registry, report *Report, step models.ScenarioStep, scenName string, idx int) {
	switch step.Action {
	case "call":
		execCall(reg, report, step, scenName, idx)
	case "sms_send":
		execSMSSend(reg, report, step, scenName, idx)
	case "sms_wait":
		execSMSWait(reg, report, step, scenName, idx)
	case "data_session":
		execDataSession(reg, report, step, scenName, idx)
	case "pause":
		log.Printf("  pause %ds", step.DurationSec)
		time.Sleep(time.Duration(step.DurationSec) * time.Second)
	case "parallel":
		log.Printf("  parallel (%d sub-steps)", len(step.Steps))
		var wg sync.WaitGroup
		for i, sub := range step.Steps {
			wg.Add(1)
			go func(s models.ScenarioStep, subIdx int) {
				defer wg.Done()
				executeStep(reg, report, s, scenName, idx*1000+subIdx)
			}(sub, i)
		}
		wg.Wait()
		log.Printf("  parallel done")
	default:
		log.Printf("  unknown action: %s", step.Action)
	}
}

// ---- Call ----
func execCall(reg *Registry, report *Report, step models.ScenarioStep, scenName string, idx int) {
	fromCfg, ok1 := reg.configs[step.FromModem]
	toCfg, ok2 := reg.configs[step.ToModem]
	caller, ok3 := reg.ctrls[step.FromModem]
	receiver, ok4 := reg.ctrls[step.ToModem]
	if !ok1 || !ok2 || !ok3 || !ok4 {
		log.Printf("  ERROR: modem not found")
		return
	}

	rec := models.CallRecord{
		ID: uuid.New().String(), Direction: "outgoing",
		FromModem: step.FromModem, NumberA: fromCfg.PhoneNumber,
		ToModem: step.ToModem, NumberB: toCfg.PhoneNumber,
		CallStart: time.Now(), Status: "initiated",
		ScenarioName: scenName, StepIndex: idx,
	}

	// Receiver goroutine
	type recvRes struct{ ok bool }
	recvCh := make(chan recvRes, 1)
	go func() {
		_, answered, _ := receiver.WaitAndAnswer(30 * time.Second)
		recvCh <- recvRes{answered}
	}()
	time.Sleep(time.Second)

	connected, _ := caller.Dial(toCfg.PhoneNumber)
	if !connected {
		rec.Status = "no_answer"
		now := time.Now(); rec.CallEnd = &now
		dur := time.Since(rec.CallStart).Seconds(); rec.TotalDurationSec = &dur
		<-recvCh
		pushCallMetrics(rec); report.AddCall(rec); return
	}

	res := <-recvCh
	if !res.ok {
		rec.Status = "failed"
		now := time.Now(); rec.CallEnd = &now
		dur := time.Since(rec.CallStart).Seconds(); rec.TotalDurationSec = &dur
		caller.Hangup()
		pushCallMetrics(rec); report.AddCall(rec); return
	}

	now := time.Now(); rec.AnswerTime = &now; rec.Status = "answered"
	log.Printf("  holding %ds", step.HoldDurationSec)
	time.Sleep(time.Duration(step.HoldDurationSec) * time.Second)

	caller.Hangup(); time.Sleep(500 * time.Millisecond); receiver.Hangup()
	end := time.Now(); rec.CallEnd = &end
	talk := end.Sub(*rec.AnswerTime).Seconds(); rec.TalkDurationSec = &talk
	total := end.Sub(rec.CallStart).Seconds(); rec.TotalDurationSec = &total
	pushCallMetrics(rec); report.AddCall(rec)
}

func pushCallMetrics(rec models.CallRecord) {
	metrics.CallsTotal.WithLabelValues(rec.FromModem, rec.ToModem, rec.Status).Inc()
	if rec.TalkDurationSec != nil {
		metrics.CallDuration.WithLabelValues(rec.FromModem, rec.ToModem).Observe(*rec.TalkDurationSec)
	}
}

// ---- SMS send ----
func execSMSSend(reg *Registry, report *Report, step models.ScenarioStep, scenName string, idx int) {
	fromCfg, ok := reg.configs[step.FromModem]
	sender, ok2 := reg.ctrls[step.FromModem]
	if !ok || !ok2 {
		log.Printf("  ERROR: sender modem not found"); return
	}

	var numberB, toModem string
	if step.ToModem != "" {
		if cfg, ok := reg.configs[step.ToModem]; ok {
			numberB = cfg.PhoneNumber; toModem = step.ToModem
		}
	} else {
		numberB = step.ToNumber
	}

	rec := models.SmsRecord{
		ID: uuid.New().String(), Direction: "outgoing",
		FromModem: step.FromModem, NumberA: fromCfg.PhoneNumber,
		ToModem: toModem, NumberB: numberB,
		Text: step.Text, TextLength: len(step.Text),
		SMSParts: models.SMSPartsCount(step.Text),
		SentAt: time.Now(), Status: "failed",
		ScenarioName: scenName, StepIndex: idx,
	}

	// Receiver goroutine for modem-to-modem
	type smsRes struct{ from, text string; ok bool }
	var recvCh chan smsRes
	if toModem != "" {
		if receiver, ok := reg.ctrls[toModem]; ok {
			recvCh = make(chan smsRes, 1)
			receiver.ClearInbox()
			go func() {
				f, t, ok, _ := receiver.WaitForSMS(60 * time.Second)
				recvCh <- smsRes{f, t, ok}
			}()
		}
	}

	ok3, err := sender.SendSMS(numberB, step.Text)
	rec.SentAt = time.Now()
	if err != nil || !ok3 {
		log.Printf("  SMS send failed: %v", err)
		if recvCh != nil { <-recvCh }
		pushSMSMetrics(rec); report.AddSMS(rec); return
	}
	rec.Status = "sent"

	if recvCh != nil {
		res := <-recvCh
		if res.ok {
			now := time.Now(); rec.ReceivedAt = &now
			dur := now.Sub(rec.SentAt).Seconds(); rec.DeliveryTimeSec = &dur
			rec.Status = "delivered"
			log.Printf("  SMS delivered in %.2fs", dur)
		} else {
			rec.Status = "timeout"
		}
	}
	pushSMSMetrics(rec); report.AddSMS(rec)
}

func pushSMSMetrics(rec models.SmsRecord) {
	metrics.SMSTotal.WithLabelValues(rec.FromModem, rec.Direction, rec.Status).Inc()
	if rec.DeliveryTimeSec != nil && rec.ToModem != "" {
		metrics.SMSDelivery.WithLabelValues(rec.FromModem, rec.ToModem).Observe(*rec.DeliveryTimeSec)
	}
}

// ---- SMS wait ----
func execSMSWait(reg *Registry, report *Report, step models.ScenarioStep, scenName string, idx int) {
	cfg, ok := reg.configs[step.FromModem]
	receiver, ok2 := reg.ctrls[step.FromModem]
	if !ok || !ok2 { log.Printf("  ERROR: modem not found"); return }

	tmo := step.TimeoutSec; if tmo <= 0 { tmo = 60 }
	receiver.ClearInbox()

	rec := models.SmsRecord{
		ID: uuid.New().String(), Direction: "incoming",
		FromModem: step.FromModem, NumberB: cfg.PhoneNumber,
		SentAt: time.Now(), Status: "timeout",
		ScenarioName: scenName, StepIndex: idx,
	}

	from, text, ok3, _ := receiver.WaitForSMS(time.Duration(tmo) * time.Second)
	if ok3 {
		now := time.Now(); rec.ReceivedAt = &now
		rec.NumberA = from; rec.Text = text; rec.TextLength = len(text)
		rec.SMSParts = models.SMSPartsCount(text); rec.Status = "received"
	}
	pushSMSMetrics(rec); report.AddSMS(rec)
}

// ---- Data session ----
func execDataSession(reg *Registry, report *Report, step models.ScenarioStep, scenName string, idx int) {
	cfg, ok := reg.configs[step.Modem]
	tableID, ok2 := reg.tableIDs[step.Modem]
	if !ok || !ok2 { log.Printf("  ERROR: modem not found"); return }

	rec := models.DataRecord{
		ID: uuid.New().String(), Modem: step.Modem,
		Number: cfg.PhoneNumber, APN: cfg.APN, Operator: cfg.Operator,
		SessionStart: time.Now(), Status: "failed",
		ScenarioName: scenName, StepIndex: idx,
	}

	session := data.NewQMISession(step.Modem, cfg.QMIDevice, cfg.NetIface,
		cfg.APN, cfg.APNUser, cfg.APNPass, tableID)

//	rxBefore, txBefore := session.GetStatus()

	if err := session.Connect(); err != nil {
		rec.ErrorMsg = err.Error()
		now := time.Now(); rec.SessionEnd = &now
		log.Printf("  data connect failed: %v", err)
		pushDataMetrics(rec); report.AddData(rec); return
	}
	rec.IPAddress = session.IPAddress

	session.WaitConnectivity(20 * time.Second)
	rxBefore, txBefore := session.GetStatus()

	for _, action := range step.Actions {
		result := data.ExecuteAction(session, action)
		rec.Actions = append(rec.Actions, result)
	}

	session.Disconnect()

	rxAfter, txAfter := session.GetStatus()
	rec.TotalBytesRx = rxAfter - rxBefore
	rec.TotalBytesTx = txAfter - txBefore
	rec.TotalBytes = rec.TotalBytesRx + rec.TotalBytesTx

	end := time.Now(); rec.SessionEnd = &end
	dur := end.Sub(rec.SessionStart).Seconds(); rec.SessionDuration = &dur
	rec.Status = "completed"

	log.Printf("  data done: %.2fs rx=%d tx=%d", dur, rec.TotalBytesRx, rec.TotalBytesTx)
	pushDataMetrics(rec); report.AddData(rec)
}

func pushDataMetrics(rec models.DataRecord) {
	metrics.DataSessions.WithLabelValues(rec.Modem, rec.Operator, rec.Status).Inc()
	metrics.DataBytesRx.WithLabelValues(rec.Modem, rec.Operator).Add(float64(rec.TotalBytesRx))
	metrics.DataBytesTx.WithLabelValues(rec.Modem, rec.Operator).Add(float64(rec.TotalBytesTx))
}

func computeSummary(r *models.ScenarioReport) {
	for _, c := range r.Calls {
		r.Summary.TotalCalls++
		if c.Status == "answered" { r.Summary.SuccessfulCalls++ } else { r.Summary.FailedCalls++ }
	}
	for _, s := range r.SMS {
		r.Summary.TotalSMS++
		if s.Status == "sent" || s.Status == "delivered" || s.Status == "received" {
			r.Summary.SuccessfulSMS++
		} else { r.Summary.FailedSMS++ }
	}
	for _, d := range r.Data {
		r.Summary.TotalData++
		if d.Status == "completed" { r.Summary.SuccessfulData++ } else { r.Summary.FailedData++ }
		r.Summary.TotalBytes += d.TotalBytes
	}
}
