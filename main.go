package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/influxdata/influxdb/client/v2"
	"log"
	"net"
	"os"
	"strconv"
	"time"
)

const (
	PORT            = 50000
	INFLUX_HOSTNAME = "localhost"
	INFLUX_PORT     = 8086
	INFLUX_USERNAME = "root"
	INFLUX_PASSWORD = "root"
	INFLUX_DBNAME   = "network"
)

var debug bool

func createLogger() log.Logger {
	logger := log.New(os.Stdout, "", 0)
	logger.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)

	return *logger
}

func debugLog(msg string) {
	if debug {
		logger := createLogger()
		logger.Println(msg)
	}
}

func influxDBClient() client.Client {
	logger := createLogger()
	c, err := client.NewHTTPClient(client.HTTPConfig{
		Addr:     fmt.Sprintf("http://%v:%d", INFLUX_HOSTNAME, INFLUX_PORT),
		Username: INFLUX_USERNAME,
		Password: INFLUX_PASSWORD,
	})

	if err != nil {
		logger.Fatalln("error: ", err)
	}

	_, ver, err := c.Ping(3)

	if err != nil {
		logger.Fatalln("error: ", err)
	} else {
		logger.Print("connected to influxdb server running version " + ver)
	}

	return c
}

func createListener() net.Listener {
	logger := createLogger()
	logger.Print("starting tcp server on port " + strconv.Itoa(PORT))

	s, err := net.Listen("tcp", ":"+strconv.Itoa(PORT))

	if err != nil {
		logger.Fatalln("error: ", err)
	}

	return s
}

func createDatabase(c client.Client) {
	logger := createLogger()
	q := client.NewQuery("CREATE DATABASE "+INFLUX_DBNAME, "", "")

	response, err := c.Query(q)

	if err != nil && response.Error() == nil {
		logger.Fatalln(err)
	} else {
		logger.Printf("created database %v", INFLUX_DBNAME)
	}
}

func clientConns(listener net.Listener) chan net.Conn {
	logger := createLogger()
	ch := make(chan net.Conn)
	i := 0
	go func() {
		for {
			client, err := listener.Accept()
			if err != nil {
				logger.Fatalln("error: ", err)
				continue
			}
			i++
			logger.Print("new connection from " + client.RemoteAddr().String())
			ch <- client
		}
	}()

	return ch
}

func parseJson(data string) interface{} {
	logger := createLogger()
	var f interface{}
	err := json.Unmarshal([]byte(data), &f)

	if err != nil {
		logger.Fatalln("error: ", err)
	}

	return f
}

func writeDataToInfluxDB(c client.Client, line []byte) {
	logger := createLogger()
	f := parseJson(string(line[:]))
	m := f.(map[string]interface{})

	bp, err := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  INFLUX_DBNAME,
		Precision: "s",
	})

	if err != nil {
		logger.Fatalln("error: ", err)
	}
	tags := map[string]string{
		"host":      m["router-id"].(string),
		"interface": m["port"].(string),
	}

	fields := map[string]interface{}{
		"rxpkt":     m["rxpkt"],
		"rxucpkt":   m["rxucpkt"],
		"rxmcpkt":   m["rxmcpkt"],
		"rxbcpkt":   m["rxbcpkt"],
		"rxpps":     m["rxpps"],
		"rxbyte":    m["rxbyte"],
		"rxbps":     m["rxbps"],
		"rxdroppkt": m["rxdroppkt"],
		"txpkt":     m["txpkt"],
		"txucpkt":   m["txucpkt"],
		"txmcpkt":   m["txmcpkt"],
		"txbcpkt":   m["txbcpkt"],
		"txpps":     m["txpps"],
		"txbyte":    m["txbyte"],
		"txbps":     m["txbps"],
		"txdroppkt": m["txdroppkt"],
	}

	point, err := client.NewPoint(
		"network_metrics",
		tags,
		fields,
		time.Now(),
	)

	if err != nil {
		logger.Fatalln("error: ", err)
	}

	bp.AddPoint(point)
	err = c.Write(bp)

	if err != nil {
		logger.Fatalln("error:", err)
	}

	debugLog(fmt.Sprintf("saved metrics for %v : %v", m["router-id"], m["port"]))
}

func handleConnection(client net.Conn) {
	logger := createLogger()
	b := bufio.NewReader(client)
	c := influxDBClient()
	for {
		line, err := b.ReadBytes('\n')
		if err != nil {
			logger.Print("disconnect " + client.RemoteAddr().String())
			break
		}
		debugLog("received json: " + string(line[:]))
		writeDataToInfluxDB(c, line)
	}
}

func main() {

	flag.BoolVar(&debug, "debug", false, "run in debug mode")
	flag.Parse()

	s := createListener()

	c := influxDBClient()

	createDatabase(c)

	conns := clientConns(s)

	for {
		go handleConnection(<-conns)
	}
}
