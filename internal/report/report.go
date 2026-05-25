package report

import (
	"call-tester/internal/models"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func SaveJSON(r *models.ScenarioReport, dir string) string {
	os.MkdirAll(dir, 0755)
	f := filepath.Join(dir, fmt.Sprintf("report_%s_%s.json", san(r.ScenarioName), r.ExecutedAt.Format("20060102_150405")))
	d, _ := json.MarshalIndent(r, "", "  ")
	os.WriteFile(f, d, 0644)
	log.Printf("JSON: %s", f)
	return f
}

func SaveCSVCalls(r *models.ScenarioReport, dir string) string {
	if len(r.Calls) == 0 { return "" }
	os.MkdirAll(dir, 0755)
	f := filepath.Join(dir, fmt.Sprintf("calls_%s_%s.csv", san(r.ScenarioName), r.ExecutedAt.Format("20060102_150405")))
	w := newCSV(f)
	defer w.f.Close()
	w.w.Write([]string{"ID","Сценарий","Шаг","Модем_А","Номер_А","Модем_Б","Номер_Б","Направление","Дата","Время_начала","Время_ответа","Время_конца","Длительность_разговора_сек","Полная_длительность_сек","Статус"})
	for _, c := range r.Calls {
		w.w.Write([]string{c.ID,c.ScenarioName,itoa(c.StepIndex),c.FromModem,c.NumberA,c.ToModem,c.NumberB,c.Direction,
			c.CallStart.Format("2006-01-02"),c.CallStart.Format("15:04:05.000"),ft(c.AnswerTime),ft(c.CallEnd),ff(c.TalkDurationSec),ff(c.TotalDurationSec),c.Status})
	}
	w.w.Flush()
	log.Printf("CSV calls: %s", f)
	return f
}

func SaveCSVSMS(r *models.ScenarioReport, dir string) string {
	if len(r.SMS) == 0 { return "" }
	os.MkdirAll(dir, 0755)
	f := filepath.Join(dir, fmt.Sprintf("sms_%s_%s.csv", san(r.ScenarioName), r.ExecutedAt.Format("20060102_150405")))
	w := newCSV(f)
	defer w.f.Close()
	w.w.Write([]string{"ID","Сценарий","Шаг","Модем_А","Номер_А","Модем_Б","Номер_Б","Направление","Дата","Время_отправки","Время_получения","Длина_текста","Частей_SMS","Время_доставки_сек","Статус"})
	for _, s := range r.SMS {
		w.w.Write([]string{s.ID,s.ScenarioName,itoa(s.StepIndex),s.FromModem,s.NumberA,s.ToModem,s.NumberB,s.Direction,
			s.SentAt.Format("2006-01-02"),s.SentAt.Format("15:04:05.000"),ft(s.ReceivedAt),itoa(s.TextLength),itoa(s.SMSParts),ff(s.DeliveryTimeSec),s.Status})
	}
	w.w.Flush()
	log.Printf("CSV SMS: %s", f)
	return f
}

func SaveCSVData(r *models.ScenarioReport, dir string) string {
	if len(r.Data) == 0 { return "" }
	os.MkdirAll(dir, 0755)
	f := filepath.Join(dir, fmt.Sprintf("data_%s_%s.csv", san(r.ScenarioName), r.ExecutedAt.Format("20060102_150405")))
	w := newCSV(f)
	defer w.f.Close()
	w.w.Write([]string{"ID","Сценарий","Шаг","Модем","Номер","Оператор","APN","IP","Дата","Начало","Конец","Длительность_сек","Байт_RX","Байт_TX","Байт_всего","МБ_всего","Статус"})
	for _, d := range r.Data {
		mb := float64(d.TotalBytes) / (1024 * 1024)
		w.w.Write([]string{d.ID,d.ScenarioName,itoa(d.StepIndex),d.Modem,d.Number,d.Operator,d.APN,d.IPAddress,
			d.SessionStart.Format("2006-01-02"),d.SessionStart.Format("15:04:05.000"),ft(d.SessionEnd),ff(d.SessionDuration),
			u64(d.TotalBytesRx),u64(d.TotalBytesTx),u64(d.TotalBytes),fmt.Sprintf("%.3f",mb),d.Status})
	}
	w.w.Flush()
	log.Printf("CSV data: %s", f)
	return f
}

func PrintSummary(r *models.ScenarioReport) {
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════╗")
	fmt.Printf("║  Отчёт: %-47s ║\n", r.ScenarioName)
	fmt.Printf("║  Выполнен: %-44s ║\n", r.ExecutedAt.Format("2006-01-02 15:04:05"))
	fmt.Println("╠══════════════════════════════════════════════════════════╣")
	fmt.Printf("║  Звонков: %d всего, %d ок, %d ошибок\n", r.Summary.TotalCalls, r.Summary.SuccessfulCalls, r.Summary.FailedCalls)
	fmt.Printf("║  SMS:     %d всего, %d ок, %d ошибок\n", r.Summary.TotalSMS, r.Summary.SuccessfulSMS, r.Summary.FailedSMS)
	fmt.Printf("║  Data:    %d всего, %d ок, %d ошибок\n", r.Summary.TotalData, r.Summary.SuccessfulData, r.Summary.FailedData)
	fmt.Printf("║  Трафик:  %.3f MB\n", float64(r.Summary.TotalBytes)/(1024*1024))
	fmt.Println("╚══════════════════════════════════════════════════════════╝")
}

// helpers
type csvW struct { f *os.File; w *csv.Writer }
func newCSV(path string) csvW {
	f, _ := os.Create(path)
	f.Write([]byte{0xEF, 0xBB, 0xBF})
	return csvW{f, csv.NewWriter(f)}
}
func san(s string) string { return strings.ReplaceAll(s, " ", "_") }
func ft(t *time.Time) string { if t == nil { return "" }; return t.Format("15:04:05.000") }
func ff(f *float64) string { if f == nil { return "" }; return fmt.Sprintf("%.2f", *f) }
func itoa(n int) string { return fmt.Sprintf("%d", n) }
func u64(n uint64) string { return fmt.Sprintf("%d", n) }
