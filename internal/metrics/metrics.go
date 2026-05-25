package metrics

import (
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	ModemRegistered = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "modem_registered",
		Help: "1 if modem registered in network",
	}, []string{"modem", "operator"})

	ModemSignalRSSI = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "modem_signal_rssi",
		Help: "RSSI (0-31, 99=none)",
	}, []string{"modem"})

	ModemSignalDBm = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "modem_signal_dbm",
		Help: "Signal in dBm",
	}, []string{"modem"})

	CallsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "calls_total",
		Help: "Total calls",
	}, []string{"from_modem", "to_modem", "status"})

	CallDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "call_duration_seconds",
		Help: "Call talk duration",
    Buckets: []float64{5, 10, 30, 60, 120, 300, 600},
	}, []string{"from_modem", "to_modem"})

	SMSTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "sms_total",
		Help: "Total SMS",
	}, []string{"from_modem", "direction", "status"})

	SMSDelivery = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "sms_delivery_seconds",
		Help: "SMs delivery time",
		Buckets: []float64{1, 3, 5, 10, 30, 60},
	}, []string{"from_modem", "to_modem"})

	DataBytesRx = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "data_bytes_rx_total",
		Help: "Bytes received",
	}, []string{"modem", "operator"})

	DataBytesTx = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "data_bytes_tx_total",
		Help: "Bytes sent",
	}, []string{"modem", "operator"})

	DataSessions = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "data_sessions_total",
		Help: "Data sessions",
	}, []string{"modem", "operator", "status"})
)

func init() {
	prometheus.MustRegister(
		ModemRegistered,
		ModemSignalRSSI,
		ModemSignalDBm,
		CallsTotal,
		CallDuration,
		SMSTotal,
		SMSDelivery,
		DataBytesRx,
		DataBytesTx,
		DataSessions,
	)
}

// Server запускает HTTP /metrics на указанном адресе
func Serve(addr string)  {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	log.Printf("metrics on %s/metrics", addr)
	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Printf("metrics server error: %v", err)
		}
	}()
}
