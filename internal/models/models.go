package models

import (
	"fmt"
	"time"
)

// ================= CONFIG ====================

type SystemConfig struct {
	Metrics MetricsConfig `yaml:"metrics"`
	Modems  []ModemConfig `yaml:"modems"`
}

type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Listen  string `yaml:"listen"`
}

type ModemConfig struct {
	Name        string `yaml:"name"`
	ATPort      string `yaml:"at_port"`
	QMIDevice   string `yaml:"qmi_device"`
	NetIface    string `yaml:"net_iface"`
	BaudRate    int    `yaml:"baud_rate"`
	Model       string `yaml:"model"`
	PhoneNumber string `yaml:"phone_number"`
	APN         string `yaml:"apn"`
	APNUser     string `yaml:"apn_user"`
	APNPass     string `yaml:"apn_pass"`
	Operator    string `yaml:"operator"`
}

// ================= SCENARIO =====================

type Scenario struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Steps       []ScenarioStep `yaml:"steps"`
}

type ScenarioStep struct {
	Action string `yaml:"action"`
	// call
	FromModem       string `yaml:"from_modem"`
	ToModem         string `yaml:"to_modem"`
	HoldDurationSec int    `yaml:"hold_duration_sec"`

	// pause
	DurationSec int `yaml:"duration_sec"`
	// sms_send
	ToNumber string `yaml:"to_number"`
	Text     string `yaml:"text"`
	// sms_wait
	TimeoutSec int `yaml:"timeout_sec"`

	// data_session
	Modem   string       `yaml:"modem"`
	Actions []DataAction `yaml:"actions"`

	// parallel
	Steps []ScenarioStep `yaml:"steps"`
}

type DataAction struct {
	Type        string `yaml:"type"` // http_get, ping, idle
	URL         string `yaml:"url"`
	Host        string `yaml:"host"`
	Count       int    `yaml:"count"`
	DurationSec int    `yaml:"duration_sec"`
	TimeoutSec  int    `yaml:"timeout_sec"`
}

// ==================== CDR: CALLS ==========================

type CallRecord struct {
	ID               string     `json:"id"`
	Direction        string     `json:"direction"`
	FromModem        string     `json:"from_modem"`
	NumberA          string     `json:"number_a"`
	ToModem          string     `json:"to_modem"`
	NumberB          string     `json:"number_b"`
	CallStart        time.Time  `json:"call_start"`
	AnswerTime       *time.Time `json:"answer_time,omitempty"`
	CallEnd          *time.Time `json:"call_end,omitempty"`
	TalkDurationSec  *float64   `json:"talk_duration_sec,omitempty"`
	TotalDurationSec *float64   `json:"total_duration_sec,omitempty"`
	Status           string     `json:"status"`
	ScenarioName     string     `json:"scenario_name"`
	StepIndex        int        `json:"step_index"`
}

// =================== CDR: SMS ===============================

type SmsRecord struct {
	ID              string     `json:"id"`
	Direction       string     `json:"direction"`
	FromModem       string     `json:"from_modem"`
	NumberA         string     `json:"number_a"`
	ToModem         string     `json:"to_modem"`
	NumberB         string     `json:"number_b"`
	Text            string     `json:"text"`
	TextLength      int        `json:"text_length"`
	SMSParts        int        `json:"sms_parts"`
	SentAt          time.Time  `json:"sent_at"`
	ReceivedAt      *time.Time `json:"received_at,omitempty"`
	DeliveryTimeSec *float64   `json:"delivery_time_sec,omitempty"`
	Status          string     `json:"status"`
	ScenarioName    string     `json:"scenario_name"`
	StepIndex       int        `json:"step_index"`
}

// ====================== CDR: DATA ===========================

type DataActionResult struct {
	ActionType  string    `json:"action_type"`
	URL         string    `json:"url,omitempty"`
	Host        string    `json:"host"`
	StartedAt   time.Time `json:"started_at"`
	EndedAt     time.Time `json:"ended_at"`
	DurationSec float64   `json:"duration_sec"`
	BytesRx     uint64    `json:"bytes_rx"`
	BytesTx     uint64    `json:"bytes_tx"`
	Success     bool      `json:"success"`
	ErrorMsg    string    `json:"error_msg,omitempty"`
	HTTPStatus  int       `json:"http_status,omitempty"`
	PingStatus  string    `json:"ping_status,omitempty"`
}

type DataRecord struct {
	ID              string             `json:"id"`
	Modem           string             `json:"modem"`
	Number          string             `json:"number"`
	APN             string             `json:"apn"`
	Operator        string             `json:"operator"`
	IPAddress       string             `json:"ip_address"`
	SessionStart    time.Time          `json:"session_start"`
	SessionEnd      *time.Time         `json:"session_end,omitempty"`
	SessionDuration *float64           `json:"session_duration_sec,omitempty"`
	TotalBytesRx    uint64             `json:"total_bytes_rx"`
	TotalBytesTx    uint64             `json:"total_bytes_tx"`
	TotalBytes      uint64             `json:"total_bytes"`
	Actions         []DataActionResult `json:"actions,omitempty"`
	Status          string             `json:"status"`
	ErrorMsg        string             `json:"error_msg,omitempty"`
	ScenarioName    string             `json:"scenario_name"`
	StepIndex       int                `json:"step_index"`
}

type Summary struct {
	TotalCalls      int    `json:"total_calls"`
	SuccessfulCalls int    `json:"successful_calls"`
	FailedCalls     int    `json:"failed_calls"`
	TotalSMS        int    `json:"total_sms"`
	SuccessfulSMS   int    `json:"successful_sms"`
	FailedSMS       int    `json:"failed_sms"`
	TotalData       int    `json:"total_data,omitempty"`
	SuccessfulData  int    `json:"successful_data_sessions"`
	FailedData      int    `json:"failed_data_sessions"`
	TotalBytes      uint64 `json:"total_bytes"`
}

type ScenarioReport struct {
	ScenarioName string       `json:"scenario_name"`
	ExecutedAt   time.Time    `json:"executrd_at"`
	Calls        []CallRecord `json:"calls"`
	SMS          []SmsRecord  `json:"sms"`
	Data         []DataRecord `json:"data"`
	Summary      Summary      `json:"summary"`
}

// ===================== UTILS ===========================

func SMSPartsCount(text string) int {
	n := len(text)
	if n <= 160 {
		return 1
	}
	return (n + 152) / 153
}

func SignalDBm(rssi int) string {
	if rssi == 99 {
		return "нет сигнала"
	}

	return fmt.Sprintf("%d dBm", -113+rssi*2)
}
