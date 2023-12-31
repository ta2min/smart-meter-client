package main

import (
	"bufio"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"
)

var ErrUnexpectedString = errors.New("unexpected string")
var ErrParse = errors.New("parse error")
var ErrReadTimeout = errors.New("read timeout")

type NetWrokInfo struct {
	Channel     string
	ChannelPage string
	PanID       string
	Addr        string
	LQI         string
	PairID      string
}

func (n NetWrokInfo) isValid() bool {
	return n.Channel != "" && n.ChannelPage != "" && n.PanID != "" && n.Addr != "" && n.LQI != "" && n.PairID != ""
}

type BP35A1 struct {
	serial.Port
	BufReader   *bufio.Reader
	NetWrokInfo NetWrokInfo
	Debug       bool
	DebugWriter io.Writer
	RouteB_ID   string
	RouteB_PW   string
	IPv6Addr    string
	used        sync.Mutex
}

func (bp *BP35A1) debugPrint(a ...any) {
	if bp.Debug {
		fmt.Fprintln(bp.DebugWriter, a...)
	}
}

func (bp *BP35A1) ReadLine() ([]byte, error) {
	var data []byte
	buf := make([]byte, 1)

	for {
		_, err := bp.Read(buf)
		if err != nil {
			return nil, err
		}

		data = append(data, buf...)
		// bp.debugPrint("data len: ", len(data), "buf: ", string(buf), "buf raw: ", buf)

		if buf[0] == 0 {
			return []byte(""), ErrReadTimeout
		}

		if len(data) >= 2 && data[len(data)-2] == '\r' && data[len(data)-1] == '\n' {
			// "\r\n"が到着したら読み込み終了
			break
		}
	}

	return data[:len(data)-2], nil
}

func (bp *BP35A1) FetchVersion() (string, error) {
	bp.Write([]byte("SKVER\r\n"))

	echoBack, err := bp.ReadLine()
	if err != nil {
		return "", err
	}
	bp.debugPrint(string(echoBack))

	version, err := bp.ReadLine()
	if err != nil {
		return "", err
	}
	bp.debugPrint(string(version))

	ok, err := bp.ReadLine()
	if err != nil {
		return "", err
	}
	bp.debugPrint(string(ok))

	return string(version), nil
}

func (bp *BP35A1) RouteBLogin() error {
	bp.Write([]byte(fmt.Sprintf("SKSETRBID %s \r\n", bp.RouteB_ID)))

	echoBack, err := bp.ReadLine()
	if err != nil {
		return err
	}
	bp.debugPrint(string(echoBack))

	ok, err := bp.ReadLine()
	if err != nil {
		return err
	}
	bp.debugPrint(string(ok))

	bp.Write([]byte(fmt.Sprintf("SKSETPWD  C %s \r\n", bp.RouteB_PW)))

	echoBack, err = bp.ReadLine()
	if err != nil {
		return err
	}
	bp.debugPrint(string(echoBack))

	ok, err = bp.ReadLine()
	if err != nil {
		return err
	}
	bp.debugPrint(string(ok))

	return nil
}

func (bp *BP35A1) SetNetWrokInfo() error {
	var netWrokInfo NetWrokInfo
	scanDuration := 5
	for {
		if scanDuration > 7 {
			return fmt.Errorf("scan retry over error")
		}
		bp.Write([]byte(fmt.Sprintf("SKSCAN 2 FFFFFFFF %d\r\n", scanDuration)))
		scanEnd := false

		for !scanEnd {
			res, err := bp.ReadLine()
			if err != nil {
				return err
			}
			if strings.HasPrefix(string(res), "EVENT 22") {
				scanEnd = true
			} else if strings.HasPrefix(string(res), "  ") {
				cols := strings.Split(strings.TrimSpace(string(res)), ":")
				bp.debugPrint(string(res))
				switch cols[0] {
				case "Channel":
					netWrokInfo.Channel = cols[1]
				case "Channel Page":
					netWrokInfo.ChannelPage = cols[1]
				case "Pan ID":
					netWrokInfo.PanID = cols[1]
				case "Addr":
					netWrokInfo.Addr = cols[1]
				case "LQI":
					netWrokInfo.LQI = cols[1]
				case "PairID":
					netWrokInfo.PairID = cols[1]
				}
			}
		}
		if netWrokInfo.isValid() {
			bp.NetWrokInfo = netWrokInfo
			return nil
		}
		scanDuration++
	}
}

func (bp *BP35A1) RegistChannel() error {
	bp.Write([]byte(fmt.Sprintf("SKSREG S2 %s\r\n", bp.NetWrokInfo.Channel)))

	echoBack, err := bp.ReadLine()
	if err != nil {
		return err
	}
	bp.debugPrint(string(echoBack))

	ok, err := bp.ReadLine()
	if err != nil {
		return err
	}
	bp.debugPrint(string(ok))

	return nil
}

func (bp *BP35A1) RegistPanID() error {
	bp.Write([]byte(fmt.Sprintf("SKSREG S3 %s\r\n", bp.NetWrokInfo.PanID)))

	echoBack, err := bp.ReadLine()
	if err != nil {
		return err
	}
	bp.debugPrint(string(echoBack))

	ok, err := bp.ReadLine()
	if err != nil {
		return err
	}
	bp.debugPrint(string(ok))

	return nil
}

func (bp *BP35A1) SetIPv6Addr() error {
	bp.Write([]byte(fmt.Sprintf("SKLL64 %s\r\n", bp.NetWrokInfo.Addr)))

	echoBack, err := bp.ReadLine()
	if err != nil {
		return err
	}
	bp.debugPrint(string(echoBack))

	line, err := bp.ReadLine()
	if err != nil {
		return err
	}
	ipv6Addr := strings.TrimRight(string(line), "\r\n")
	bp.debugPrint(fmt.Sprintf("IP v6 Addr: %s", ipv6Addr))

	bp.IPv6Addr = ipv6Addr
	return nil
}

func (bp *BP35A1) ConBRoute() error {
	bp.Write([]byte(fmt.Sprintf("SKJOIN %s\r\n", bp.IPv6Addr)))

	echoBack, err := bp.ReadLine()
	if err != nil {
		return err
	}
	bp.debugPrint(string(echoBack))

	ok, err := bp.ReadLine()
	if err != nil {
		return err
	}
	bp.debugPrint(string(ok))

	connected := false
	for !connected {
		resByte, err := bp.ReadLine()
		if err != nil {
			return err
		}
		res := string(resByte)
		if strings.HasPrefix(res, "EVENT 24") {
			return fmt.Errorf("PANA authentication failed")
		} else if strings.HasPrefix(string(res), "EVENT 25") {
			connected = true
			bp.debugPrint("successful PANA authentication")
		}
	}

	instanceList, err := bp.ReadLine()
	if err != nil {
		return err
	}
	bp.debugPrint(string(instanceList))

	return nil
}

func (bp *BP35A1) GetMeasuredInstantaneous() (int, error) {
	bp.used.Lock()
	defer bp.used.Unlock()

	echonetLiteFame := []byte("\x10\x81\x00\x01\x05\xFF\x01\x02\x88\x01\x62\x01\xE7\x00")
	command := append([]byte(fmt.Sprintf("SKSENDTO 1 %s 0E1A 1 %04X ", bp.IPv6Addr, len(echonetLiteFame))), echonetLiteFame...)
	bp.debugPrint(hex.EncodeToString(command))
	bp.Write(command)

	line, err := bp.ReadLine()
	if err != nil {
		return 0, err
	}
	// エコーバック
	bp.debugPrint(string(line))

	event21, err := bp.ReadLine()
	if err != nil {
		return 0, err
	}
	bp.debugPrint(string(event21))
	if string(event21) == "EVENT 21" {
		return 0, ErrUnexpectedString
	}

	ok, err := bp.ReadLine()
	if err != nil {
		return 0, err
	}
	bp.debugPrint(string(ok))

	erxudp, err := bp.ReadLine()
	if err != nil {
		return 0, err
	}
	bp.debugPrint(string(erxudp))

	if !strings.HasPrefix(string(erxudp), "ERXUDP") {
		return 0, ErrUnexpectedString
	}

	cols := strings.Split(strings.TrimSpace(string(erxudp)), " ")
	bp.debugPrint("cols: ", cols)
	res := cols[8]
	seoj := res[8 : 8+6]
	ESV := res[20 : 20+2]
	EPC := res[24 : 24+2]
	bp.debugPrint("seoj: ", seoj, "ESV: ", ESV, "EPC: ", EPC)

	if seoj != "028801" || ESV != "72" || EPC != "E7" {
		return 0, ErrParse
	}

	r := string(erxudp)
	mi, err := bp.parseMeasuredInstantaneous(r[len(r)-8:])
	if err != nil {
		return 0, ErrParse
	}
	bp.debugPrint(fmt.Sprintf("瞬間電力計測値: %d", mi))
	return mi, nil
}

func (bp *BP35A1) parseMeasuredInstantaneous(hex string) (int, error) {
	mi, err := strconv.ParseInt(hex, 16, 64)
	if err != nil {
		return 0, err
	}
	return int(mi), nil
}

func (bp *BP35A1) GetCumulativeElectricEnergyUnit() (float64, error) {
	bp.used.Lock()
	defer bp.used.Unlock()

	UnitFrame := []byte("\x10\x81\x00\x01\x05\xFF\x01\x02\x88\x01\x62\x01\xE1\x00")
	command := append([]byte(fmt.Sprintf("SKSENDTO 1 %s 0E1A 1 %04X ", bp.IPv6Addr, len(UnitFrame))), UnitFrame...)
	bp.Write(command)

	line, err := bp.ReadLine()
	if err != nil {
		return 0, err
	}
	// エコーバック
	bp.debugPrint(string(line))

	event21, err := bp.ReadLine()
	if err != nil {
		return 0, err
	}
	bp.debugPrint(string(event21))
	if string(event21) == "EVENT 21" {
		return 0, ErrUnexpectedString
	}

	ok, err := bp.ReadLine()
	if err != nil {
		return 0, err
	}
	bp.debugPrint(string(ok))

	erxudp, err := bp.ReadLine()
	if err != nil {
		return 0, err
	}
	bp.debugPrint(string(erxudp))

	if !strings.HasPrefix(string(erxudp), "ERXUDP") {
		return 0, ErrUnexpectedString
	}

	cols := strings.Split(strings.TrimSpace(string(erxudp)), " ")
	bp.debugPrint("cols: ", cols)
	res := cols[8]
	seoj := res[8 : 8+6]
	ESV := res[20 : 20+2]
	EPC := res[24 : 24+2]
	bp.debugPrint("seoj: ", seoj, "ESV: ", ESV, "EPC: ", EPC)

	if seoj != "028801" || ESV != "72" || EPC != "E1" {
		return 0, ErrParse
	}

	r := string(erxudp)
	unit, err := bp.parseCumulativeElectricEnergyUnit(r[len(r)-2:])
	if err != nil {
		return 0, err
	}

	bp.debugPrint(fmt.Sprintf("積算電力量単位: %fkWh", unit))
	return unit, nil
}

func (bp *BP35A1) parseCumulativeElectricEnergyUnit(data string) (float64, error) {
	u, err := strconv.ParseInt(data, 16, 64)
	if err != nil {
		return 0, err
	}
	var unit float64
	switch u {
	case 0:
		unit = 1
	case 1:
		unit = 0.1
	case 2:
		unit = 0.01
	case 3:
		unit = 0.001
	case 4:
		unit = 0.0001
	case 10:
		unit = 10
	case 11:
		unit = 100
	case 12:
		unit = 1000
	case 13:
		unit = 10000
	default:
		bp.debugPrint("inccorect number: ", u)
		return 0, ErrParse
	}
	return unit, nil
}

func (bp *BP35A1) GetRegularTimeNormalDirectionCumulativeElectricEnergy() (int, *time.Time, error) {
	bp.used.Lock()
	defer bp.used.Unlock()

	cumulativeElectricEnergyFrame := []byte("\x10\x81\x00\x01\x05\xFF\x01\x02\x88\x01\x62\x01\xEA\x00")
	command := append([]byte(fmt.Sprintf("SKSENDTO 1 %s 0E1A 1 %04X ", bp.IPv6Addr, len(cumulativeElectricEnergyFrame))), cumulativeElectricEnergyFrame...)
	bp.Write(command)

	line, err := bp.ReadLine()
	if err != nil {
		return 0, nil, err
	}
	bp.debugPrint(string(line))

	event21, err := bp.ReadLine()
	if err != nil {
		return 0, nil, err
	}
	bp.debugPrint(string(event21))
	if string(event21) == "EVENT 21" {
		return 0, nil, ErrUnexpectedString
	}

	ok, err := bp.ReadLine()
	if err != nil {
		return 0, nil, err
	}
	bp.debugPrint(string(ok))

	erxudp, err := bp.ReadLine()
	if err != nil {
		return 0, nil, err
	}
	bp.debugPrint(string(erxudp))

	if !strings.HasPrefix(string(erxudp), "ERXUDP") {
		return 0, nil, ErrUnexpectedString
	}

	cols := strings.Split(strings.TrimSpace(string(erxudp)), " ")
	bp.debugPrint("cols: ", cols)
	res := cols[8]
	seoj := res[8 : 8+6]
	ESV := res[20 : 20+2]
	EPC := res[24 : 24+2]
	bp.debugPrint("seoj: ", seoj, "ESV: ", ESV, "EPC: ", EPC)

	if seoj != "028801" || ESV != "72" || EPC != "EA" {
		return 0, nil, ErrParse
	}

	r := string(erxudp)
	cee, time, err := bp.parseRegularTimeNormalDirectionCumulativeElectricEnergy(r[len(r)-22:])
	if err != nil {
		return 0, nil, err
	}

	bp.debugPrint("定時: ", time)
	bp.debugPrint("積算電力量: ", cee)

	return int(cee), time, nil
}

func (bp *BP35A1) parseRegularTimeNormalDirectionCumulativeElectricEnergy(data string) (int, *time.Time, error) {
	tmp := data[:4]
	yy, _ := strconv.ParseInt(tmp, 16, 64)
	tmp = data[4 : 4+2]
	MM, _ := strconv.ParseInt(tmp, 16, 64)
	tmp = data[6 : 6+2]
	dd, _ := strconv.ParseInt(tmp, 16, 64)

	tmp = data[8 : 8+2]
	hh, _ := strconv.ParseInt(tmp, 16, 64)
	tmp = data[10 : 10+2]
	mm, _ := strconv.ParseInt(tmp, 16, 64)
	tmp = data[12 : 12+2]
	ss, _ := strconv.ParseInt(tmp, 16, 64)
	time, err := time.Parse("20060102150405", fmt.Sprintf("%04d%02d%02d%02d%02d%02d", yy, MM, dd, hh, mm, ss))
	if err != nil {
		return 0, nil, err
	}

	tmp = data[14:]
	cumulativeElectricEnergy, err := strconv.ParseInt(tmp, 16, 64)
	if err != nil {
		return 0, nil, ErrParse
	}

	return int(cumulativeElectricEnergy), &time, nil
}

func (bp *BP35A1) GetUnitAndRegularTimeNormalDirectionCumulativeElectricEnergy() (int, *time.Time, float64, error) {
	bp.used.Lock()
	defer bp.used.Unlock()

	cumulativeElectricEnergyAndUnitFrame := []byte("\x10\x81\x00\x01\x05\xFF\x01\x02\x88\x01\x62\x02\xE1\x00\xEA\x00")
	command := append([]byte(fmt.Sprintf("SKSENDTO 1 %s 0E1A 1 %04X ", bp.IPv6Addr, len(cumulativeElectricEnergyAndUnitFrame))), cumulativeElectricEnergyAndUnitFrame...)
	bp.Write(command)

	line, err := bp.ReadLine()
	if err != nil {
		return 0, nil, 0, err
	}
	bp.debugPrint(string(line))

	event21, err := bp.ReadLine()
	if err != nil {
		return 0, nil, 0, err
	}
	bp.debugPrint(string(event21))
	if string(event21) == "EVENT 21" {
		return 0, nil, 0, ErrUnexpectedString
	}

	ok, err := bp.ReadLine()
	if err != nil {
		return 0, nil, 0, err
	}
	bp.debugPrint(string(ok))

	erxudp, err := bp.ReadLine()
	if err != nil {
		return 0, nil, 0, err
	}
	bp.debugPrint(string(erxudp))

	if !strings.HasPrefix(string(erxudp), "ERXUDP") {
		return 0, nil, 0, ErrUnexpectedString
	}

	cols := strings.Split(strings.TrimSpace(string(erxudp)), " ")
	bp.debugPrint("cols: ", cols)
	res := cols[8]
	seoj := res[8 : 8+6]
	ESV := res[20 : 20+2]
	EPC1 := res[24 : 24+2]
	EPC2 := res[30 : 30+2]
	bp.debugPrint("seoj: ", seoj, "ESV: ", ESV, "EPC1: ", EPC1, "EPC2: ", EPC2)

	if seoj != "028801" || ESV != "72" || EPC1 != "E1" || EPC2 != "EA" {
		return 0, nil, 0, ErrParse
	}

	unit, err := bp.parseCumulativeElectricEnergyUnit(res[28 : 28+2])
	if err != nil {
		return 0, nil, 0, err
	}
	bp.debugPrint("unit: ", unit)
	bp.debugPrint("res: ", res[34:34+22])

	cee, time, err := bp.parseRegularTimeNormalDirectionCumulativeElectricEnergy(res[34 : 34+22])
	if err != nil {
		return 0, nil, 0, err
	}
	bp.debugPrint("定時: ", time)
	bp.debugPrint("積算電力量: ", cee)

	return cee, time, unit, nil
}

func NewBP35A1(portName string, baudRate int, RBID string, RBPW string, debugMode bool) (*BP35A1, error) {
	port, err := serial.Open(portName, &serial.Mode{
		BaudRate: baudRate,
		DataBits: 8,
	})
	if err != nil {
		return nil, err
	}

	r := bufio.NewReader(port)

	BP35A1 := &BP35A1{
		Port:        port,
		BufReader:   r,
		Debug:       debugMode,
		DebugWriter: os.Stdout,
		RouteB_ID:   RBID,
		RouteB_PW:   RBPW,
	}
	return BP35A1, nil
}

func main() {
	portNmae := flag.String("p", "", "ポート名")
	RBID := flag.String("i", "", "Bルート認証ID")
	RBPW := flag.String("P", "", "Bルート認証パスワード")
	debugMode := flag.Bool("d", false, "デバッグモード")
	flag.Parse()

	if *portNmae == "" {
		fmt.Fprintf(os.Stderr, "ポート名を入力してください\n")
		os.Exit(1)
	}

	if *RBID == "" {
		fmt.Fprintf(os.Stderr, "Bルート認証IDを入力してください\n")
		os.Exit(1)
	}

	if *RBPW == "" {
		fmt.Fprintf(os.Stderr, "Bルート認証パスワードを入力してください\n")
		os.Exit(1)
	}

	BP35A1, err := NewBP35A1(*portNmae, 115200, *RBID, *RBPW, *debugMode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ポートに接続できませんでした\n%s\n", err)
		os.Exit(1)
	}
	defer BP35A1.Close()

	BP35A1.ResetOutputBuffer()
	BP35A1.ResetInputBuffer()

	err = BP35A1.RouteBLogin()
	if err != nil {
		fmt.Println(err)
	}

	err = BP35A1.SetNetWrokInfo()
	if err != nil {
		fmt.Println(err)
	}

	err = BP35A1.RegistChannel()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("finish regist channel")

	err = BP35A1.RegistPanID()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("finish regist pan id")

	err = BP35A1.SetIPv6Addr()
	if err != nil {
		fmt.Println(err)
	}

	err = BP35A1.ConBRoute()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("successful connection to B route")

	BP35A1.SetReadTimeout(10 * time.Second)

	measuredInstantaneousTicker := time.NewTicker(1 * time.Second)
	EnergyTicker := time.NewTicker(10 * time.Second)

	for {
		select {
		case <-measuredInstantaneousTicker.C:
			go func() {
				measuredInstantaneous, err := BP35A1.GetMeasuredInstantaneous()
				if err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
				fmt.Println("瞬間電力量", measuredInstantaneous, "w")

			}()
		case <-EnergyTicker.C:
			go func() {
				cee, t, unit, err := BP35A1.GetUnitAndRegularTimeNormalDirectionCumulativeElectricEnergy()
				if err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
				fmt.Println("計測時間: ", t)
				fmt.Println("積算電力量: ", float64(cee)*float64(unit), "kwh")
			}()
		}
	}
}
