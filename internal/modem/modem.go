package modem

import (
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"go.bug.st/serial"
)

const (
	DefaultTimeout   = 5 * time.Second
	CallSetupTimeout = 60 * time.Second
	SMSSendTimeout   = 30 * time.Second
)

type Controller struct {
	port     serial.Port
	portName string
	Name     string
	Phone    string
	Model    string
	Operator string
	// QMI
	QMIDevice string
	NetIface  string
	APN       string
	APNUser   string
	APNPass   string
}

func New(portName string, baudRate int, name, phone, model, operator string) (*Controller, error) {
	mode := &serial.Mode{
		BaudRate: baudRate,
		DataBits: 8,
		StopBits: serial.OneStopBit,
		Parity:   serial.NoParity,
	}
	port, err := serial.Open(portName, mode)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", portName, err)
	}
	port.SetReadTimeout(100 * time.Microsecond)
	log.Printf("[%s] port %s opened", name, portName)
	return &Controller{
		port:     port,
		portName: portName,
		Name:     name,
		Phone:    phone,
		Model:    model,
		Operator: operator,
	}, nil
}

func (c *Controller) Close() error { return c.port.Close() }

// ---------- Low-level AT -------------

func (c *Controller) SendCommand(cmd string, timeout time.Duration) (string, error) {
	c.flushInput()
	log.Printf("[%s] >>> %s", c.Name, cmd)
	if _, err := c.port.Write([]byte(cmd + "\r")); err != nil {
		return "", err
	}
	resp, err := c.readResponse(timeout)
	if err != nil {
		return "", err
	}
	log.Printf("[%s] <<< %s", c.Name, strings.TrimSpace(resp))
	return resp, nil
}

func (c *Controller) SendCommandOk(cmd string, timeout time.Duration) (string, error) {
	resp, err := c.SendCommand(cmd, timeout)
	if err != nil {
		return "", err
	}
	if strings.Contains(resp, "OK") {
		return resp, nil
	}
	if strings.Contains(resp, "ERROR") {
		return "", fmt.Errorf("'%s' error: %s", cmd, strings.TrimSpace(resp))
	}
	return "", fmt.Errorf("'%s' unexpected: %s", cmd, strings.TrimSpace(resp))
}

func (c *Controller) SendRaw(data []byte) error {
	_, err := c.port.Write(data)
	return err
}

func (c *Controller) WaitFor(marker string, timeout time.Duration) (string, error) {
	var resp strings.Builder
	buf := make([]byte, 4096)
	start := time.Now()
	for {
		if time.Since(start) > timeout {
			return resp.String(), fmt.Errorf("timeout waiting for '%s'", marker)
		}

		n, _ := c.port.Read(buf)
		
		if n > 0 {
			resp.Write(buf[:n])
			if strings.Contains(resp.String(), marker) {
				return resp.String(), nil
			}
		} else {
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func (c *Controller) ReadURC(timeout time.Duration) string {
	var resp strings.Builder
	buf := make([]byte, 4096)
	start := time.Now()
	for {
		if time.Since(start) > timeout {
			return resp.String()
		}

		n, _ := c.port.Read(buf)
		if n > 0 {
			resp.Write(buf[:n])
			s := resp.String()
			if strings.Contains(s, "RING") || strings.Contains(s, "+CLIP") ||
			   strings.Contains(s, "+CMTI") || strings.Contains(s, "+CMT:") ||
				 strings.Contains(s, "NO CARRIER") || strings.Contains(s, "BUSY") {
         break
			}
		} else {
			time.Sleep(50 * time.Millisecond)
		}
	}
	return resp.String()
}

func (c *Controller) readResponse(timeout time.Duration) (string, error) {
	var resp strings.Builder
	buf := make([]byte, 4096)
	start := time.Now()
	for {
		if time.Since(start) > timeout {
			if resp.Len() == 0 {
				return "", fmt.Errorf("read timeout on %s", c.portName)
			}
			break
		}
		n, _ := c.port.Read(buf)
		if n > 0 {
			resp.Write(buf[:n])
			s := resp.String()
			if strings.Contains(s, "OK\r") || strings.Contains(s, "OK\n") ||
			   strings.Contains(s, "ERROR") || strings.Contains(s, "NO CARRIER") ||
				 strings.Contains(s, "BUSY") || strings.Contains(s, "NO ANSWER") ||
				 strings.Contains(s, "NO DIALTONE") {
         break
			}
		} else {
			time.Sleep(50 * time.Millisecond)
		}
	}
	return resp.String(), nil
}

func (c *Controller) flushInput() {
	buf := make([]byte, 4096)
	for {
		n, err := c.port.Read(buf)
		if n == 0 || err == io.EOF {
			break
		}
	}
}

// ----------- Init --------------

func (c *Controller) Init() error {
	log.Printf("[%s] init (%s, %s)", c.Name, c.Model, c.Operator)
	c.SendCommandOk("ATE0", DefaultTimeout)
	if _, err := c.SendCommandOk("AT", DefaultTimeout); err != nil {
		return err
	}
	resp, _ := c.SendCommand("AT+CREG?", DefaultTimeout)
	if !strings.Contains(resp, ",1") && !strings.Contains(resp, ",5") {
		log.Printf("[%s] WARNING: not registered", c.Name)
	}
	c.SendCommandOk("AT+CLIP=1", DefaultTimeout)
	c.SendCommandOk("AT+CMEE=2", DefaultTimeout)
	c.SendCommandOk("AT+CMGF=1", DefaultTimeout)
	c.SendCommandOk(`AT+CSCS="GSM"`, DefaultTimeout)
	c.SendCommandOk("AT+CNMI=2,1,0,0,0", DefaultTimeout)
	log.Printf("[%s] ready", c.Name)
	return nil
}

func (c *Controller) GetSignalQuality() (rssi, ber int, err error) {
	resp, err := c.SendCommand("AT+CSQ", DefaultTimeout)
	if err != nil {
		return 0, 0, err
	}
	for _, line := range strings.Split(resp, "\n") {
		if strings.Contains(line, "+CSQ:") {
			parts := strings.Split(strings.TrimSpace(strings.Split(line, ":")[1]), ",")
			if len(parts) >= 2 {
				fmt.Sscanf(parts[0], "%d", &rssi)
				fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &ber)
				return
			}
		}
	}
	return 0, 0, fmt.Errorf("parse CSQ failed")
}

// -------- Voice ------------

func (c *Controller) Dial(number string) (bool, error) {
	log.Printf("[%s] dialing %s", c.Name, number)
	c.SendCommand(fmt.Sprintf("ATD%s;", number), DefaultTimeout)
	start := time.Now()
	for {
		if time.Since(start) > CallSetupTimeout {
			c.Hangup()
			return false, nil
		}
		urc := c.ReadURC(2 * time.Second)
		if strings.Contains(urc, "NO CARRIER") || strings.Contains(urc, "BUSY") ||
		   strings.Contains(urc, "NO ANSWER") || strings.Contains(urc, "NO DIALTONE") {
       return false, nil
		}
		clcc, err := c.SendCommand("AT+CLCC", DefaultTimeout)
		if err == nil && strings.Contains(clcc, ",0,0,") {
			log.Printf("[%s] call established", c.Name)
			return true, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (c *Controller) WaitAndAnswer(timeout time.Duration) (caller string, ok bool, err error) {
	log.Printf("[%s] waiting for incoming call (%v)", c.Name, timeout)
	start := time.Now()
	for {
		if time.Since(start) > timeout {
			return "", false, nil
		}
		urc := c.ReadURC(2 * time.Second)
		if strings.Contains(urc, "RING") {
			caller = parseCLIP(urc)
			time.Sleep(500 * time.Millisecond)
			resp, err := c.SendCommand("ATA", DefaultTimeout)
			if err != nil {
				return caller, false, err
			}
			if strings.Contains(resp, "OK") || strings.Contains(resp, "CONNECT") {
				log.Printf("[%s] answer from %T+s", c.Name, caller)
				return caller, true, nil
			}
		}
	}
}

func (c *Controller) Hangup() {
	c.SendCommand("ATH", DefaultTimeout)
	c.SendCommand("AT+CHUP", DefaultTimeout)
}

func parseCLIP(urc string) string {
	for _, line := range strings.Split(urc, "\n") {
		if !strings.Contains(line, "+CLIP:") {
			continue
		}
		if _, after, found := strings.Cut(line, "\""); found {
			if before, _, found := strings.Cut(after, "\""); found {
				return before
			} 
		}
	}
	return "unknown"
}

// ------------- SMS ----------------

func (c *Controller) SendSMS(number, text string) (bool, error) {
	log.Printf("[%s] SMS to %s (%d chars)", c.Name, number, len(text))
	c.SendCommandOk("AT+CMGF=1", DefaultTimeout)

	cmd := fmt.Sprintf(`AT+CMGS="%s"`, number)
	c.flushInput()
	c.port.Write([]byte(cmd + "\r"))

  if _, err := c.WaitFor(">", 5*time.Second); err != nil {
		return false, fmt.Errorf("no '>' promt: %w", err)    
	}

	payload := append([]byte(text), 0x1A)
	if err := c.SendRaw(payload); err != nil {
		return false, err
	}

	resp, err := c.WaitFor("+CMGS", SMSSendTimeout)
	if err != nil {
		if strings.Contains(resp, "ERROR") {
			return false, fmt.Errorf("SMS error: %s", strings.TrimSpace(resp))
		}
		return false, err 
	}
	c.WaitFor("OK", 5*time.Second)
	log.Printf("[%s] SMS sent", c.Name)
	return true, nil
}

func (c *Controller) WaitForSMS(timeout time.Duration) (from, text string, ok bool, err error) {
	log.Printf("[%s] waiting for SMS (%v)", c.Name, timeout)
	start := time.Now()

	for {
		if time.Since(start) > timeout {
			return "", "", false, nil
		}
		urc := c.ReadURC(2 * time.Second)
		if strings.Contains(urc, "+CMTI:") {
			idx := parseCMTI(urc)
			if idx < 0 {
         continue
			}
			from, text, err = c.ReadSMS(idx)
			if err != nil {
				return "", "", false, err
			}
			c.SendCommand(fmt.Sprintf("AT+CMGD=%d", idx), DefaultTimeout)
			return from, text, true, nil
	  }
	}
}


func (c *Controller) ReadSMS(index int) (from, text string, err error) {
	resp, err := c.SendCommand(fmt.Sprintf("AT+CMGR=%d", index), DefaultTimeout)
	if err != nil {
		return "", "", err
	}
	lines := strings.Split(resp, "\n")
	for i, line := range lines {
		if strings.Contains(line, "+CMGR:") {
			parts := strings.Split(line, ",")
			if len(parts) >= 2 {
				from = strings.Trim(strings.TrimSpace(parts[1]), "\"")
			}
			if i+1 < len(lines) {
				text = strings.TrimSpace(lines[i+1])
			}
			return
		}
	}
	return "", "", fmt.Errorf("parse SMS failed")
}

func (c *Controller) ClearInbox() {
	c.SendCommand("AT+CMGD=1,4", DefaultTimeout)
}

func parseCMTI(urc string) int {
	for _, line := range strings.Split(urc, "\n") {
		if !strings.Contains(line, "+CMTI:") {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) >= 2 {
			var idx int
			fmt.Scanf(strings.TrimSpace(parts[1]), "%d", &idx)
			return idx
		}
	}
	return -1
}


