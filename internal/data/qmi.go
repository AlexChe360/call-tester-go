package data

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

type QMISession struct {
	Modem			string
	QMIDevice string
	NetIface  string
	APN				string
	APNUser   string
	APNPass   string
	TableID	  int
	IPAddress string
	connected bool
}

func NewQMISession(modem, qmiDev, iface, apn, user, pass string, tableID int) *QMISession {
	return &QMISession{
		Modem: modem,
		QMIDevice: qmiDev,
		NetIface: iface,
		APN: apn,
		APNUser: user,
		APNPass: pass,
		TableID: tableID,
	}
}

func (q *QMISession) Connect() error {
	log.Printf("[%s] QMI connect (APN=%s)", q.Modem, q.APN)

	q.sh("ip link set %s down", q.NetIface)
	q.sh("bash -c 'echo Y > /sys/class/net/%s/qmi/raw_ip'", q.NetIface)
	q.sh("ip link set %s up", q.NetIface)

	apnStr := fmt.Sprintf("apn=%s,ip-type=4", q.APN)
	if q.APNUser != "" {
		apnStr = fmt.Sprintf("apn=%s,username=%s,password=%s,auth=both,ip-type=4", q.APN, q.APNUser, q.APNPass)
	}

	out, err := q.shOut("qmicli -d %s --wds-start-network='%s' --client-no-release-cid", q.QMIDevice, apnStr)
	if err != nil || !strings.Contains(out, "Network started") {
		return fmt.Errorf("qmicli start failed: %s %v", out, err)
	}
  
	q.sh("pkill -f 'udhcpc.*%s' 2>/dev/null || true", q.NetIface)
	q.shOut("udhcpc -i %s -q -n -t 5", q.NetIface)
	time.Sleep(500 * time.Millisecond)

	ipOut, _ := q.shOut("ip -4 addr show %s | awk '/inet / {print $2}' | cut -d/ -f1", q.NetIface)
	q.IPAddress = strings.TrimSpace(ipOut)
	if q.IPAddress == "" {
		return fmt.Errorf("no IP on %s", q.NetIface)
	}
	log.Printf("[%s] QMI: IP=%s", q.Modem, q.IPAddress)

	// Policy routing
	qwOut, _ := q.shOut("ip route show dev %s | awk '/default/ {print $3}' | head -1", q.NetIface)
	qw := strings.TrimSpace(qwOut)
	q.sh("ip route flush table %d 2>/dev/null || true", q.TableID)
	if qw != "" {
		q.sh("ip route add default via %s dev %s table %d", qw, q.NetIface, q.TableID)
	} else {
		q.sh("ip route add default dev %s table %d", q.NetIface, q.TableID)
	}
	q.sh("ip rule del from %s table %d 2>/dev/null || true", q.IPAddress, q.TableID)
	q.sh("ip rule add from %s table %d", q.IPAddress, q.TableID)

	q.connected = true
	return nil
}

func (q *QMISession) Disconnect() {
	if !q.connected {
		return
	}
	log.Printf("[%s] QMI disconnect", q.Modem)
	q.sh("pkill -f 'udhcpc.*%s' 2>/dev/null || true", q.NetIface)
	q.sh("qmicli -d --wds-stop=network=disable-autoconnect 2?/dev/null || true", q.NetIface)
	if q.IPAddress != "" {
		q.sh("ip rule del from $s table %d 2>/dev/null || true", q.IPAddress, q.TableID)
	}
	q.sh("ip route flush table %d 2>/dev/null || true", q.TableID)
	q.sh("ip addr flush dev %s 2>/dev/null || true", q.NetIface)
	q.connected = false
}

func (q *QMISession) GetStatus() (rx, tx uint64) {
	rxOut, _ := q.shOut("cat /sys/class/net/%s/statistics/rx_bytes", q.NetIface)
	txOut, _ := q.shOut("cat /sys/class/net/%s/statistics/tx_bytes", q.NetIface)
	fmt.Sscanf(strings.TrimSpace(rxOut), "%d", &rx)
	fmt.Sscanf(strings.TrimSpace(txOut), "%d", &tx)
	return
}

func (q *QMISession) WaitConnectivity(timeout time.Duration) error {
	start := time.Now()
	for {
		if time.Since(start) > timeout {
			return fmt.Errorf("no internet via %s", q.NetIface)
		}
		if q.sh("ping -c 1 -W 2 -I %s 8.8.8.8 > /dev/null 2>&1", q.NetIface) == nil {
			log.Printf("[%s] internet OK", q.Modem)
			return nil
		}
		time.Sleep(time.Second)
	}
}

func (q *QMISession) sh(format string, args ...interface{}) error {
	cmd := fmt.Sprintf(format, args...)
	return exec.Command("bash", "-c", cmd).Run()
}

func (q *QMISession) shOut(format string, args ...interface{}) (string, error) {
	cmd := fmt.Sprintf(format, args...)
	c := exec.Command("bash", "-c", cmd)
	c.Env = append(c.Environ(), "LC_ALL=C")
	out, err := c.Output()
	return string(out), err
}
