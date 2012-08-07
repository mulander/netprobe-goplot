package main

import (
	"encoding/json"
	"errors"
	"expvar"
	"flag"
	"fmt"
	. "goplot/constants"
	_ "goplot/httplog"
	"io/ioutil"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
)

type Point struct {
	x float64
	y float64
}

type Config struct {
	Address   string
	CustomLog string
	LogFormat []string
}

func (pt *Point) String() string { return fmt.Sprintf("(%f,%f)", pt.x, pt.y) }

func (pt *Point) ServeHTTP(c http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET":
		pt.x++
	case "POST":
		pt.x, _ = strconv.ParseFloat(req.FormValue("x"), 64)
		pt.y, _ = strconv.ParseFloat(req.FormValue("y"), 64)
	}
	fmt.Fprintf(c, "point is (%f,%f)\n", pt.x, pt.y)
}

var configFlag = flag.String("c", "server.conf", "Config file name")
var helpFlag = flag.Bool("h", false, "This help")

// next variables are also available in server config file
var addressFlag = flag.String("l", "0.0.0.0:6060", "Address and port to listen on (ex. 127.0.0.1:1234")

func main() {
	// todo: config file overrides command line flags, this feels incorrect
	flag.Parse()

	if *helpFlag {
		flag.PrintDefaults()
		os.Exit(EXIT_SUCCESS)
	}

	configJsonBytes, err := ioutil.ReadFile(*configFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read %s: %s\n", *configFlag, err.Error())
		os.Exit(EXIT_NO_CONFIG)
	}

	var config = Config{*addressFlag, "nolog", nil}
	err = json.Unmarshal(configJsonBytes, &config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config error at %s (while reading %s)\n", strconv.Quote(err.Error()), *configFlag)
		os.Exit(EXIT_CONFIG_PARSE)
	}

	fmt.Printf("%s\n", config.Address)
	fmt.Printf("%s\n", config.CustomLog)

	demoPoint := new(Point)
	demoPoint.x = 0.0
	demoPoint.y = 0.0

	http.Handle("/point", demoPoint)
	expvar.Publish("point", demoPoint)

	http.Handle("/goplot/viz", http.HandlerFunc(dataSampleServer))
	// serve our own files instead of using http.FileServer for very tight access control
	http.Handle("/goplot/graph.js", http.HandlerFunc(fileServe))
	// in order
	err = http.ListenAndServe(config.Address, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ListenAndServe on %s got: %s\n", config.Address, err.Error())
		os.Exit(EXIT_CANT_LISTEN)
	}
}

// serve static files as appropriate
func fileServe(c http.ResponseWriter, req *http.Request) {
	cwd, err := os.Getwd()
	if err == nil {
		http.ServeFile(c, req, cwd+"/client/graph.js")
	} else {
		serveError(c, req, http.StatusInternalServerError) // 500
	}
}

// Send the given error code.
func serveError(c http.ResponseWriter, req *http.Request, code int) {
  c.WriteHeader(code)
}

// processes data samples, sends back data to plot along with regression lines
func dataSampleServer(c http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET":
		cwd, err := os.Getwd()
		if err == nil {
			http.ServeFile(c, req, cwd+"/client/viz.html")
		} else {
			serveError(c, req, http.StatusInternalServerError) // 500
		}
	case "POST":
		src := req.FormValue("dataseries")
		result := dataSampleProcess(src)
		// send the response
    fmt.Fprint(c,result)
	default:
		serveError(c, req, http.StatusMethodNotAllowed)
	}
}

// processes data samples, sends back data to plot along with regression lines
func dataSampleProcess(src string) (results string) {
	const MAXLINES = 1000000

	// split the buffer into an array of strings, one per source line
	srcLines := strings.SplitN(src, "\n", MAXLINES)

	lineCount := len(srcLines)
	series := make([]Point, 0)

	for ix := 0; ix < lineCount; ix++ {
		stmp, err := parseLine(srcLines[ix])
		if err == nil {
      series = append(series, stmp)
		}
	}
	jsonStr := "{series:["
	for ix := 0; ix < len(series); ix++ {
		jsonStr += "{x:" + strconv.FormatFloat(Point(series[ix]).x, 'f', 3, 64) + ",y:" + strconv.FormatFloat(Point(series[ix]).y, 'f', 3, 64) + "},"
	}
	jsonStr += "],\n"

	slope, intercept, stdError, correlation := linearRegression(series)
	jsonStr += fmt.Sprintf("regressionLine:{slope:%f,intercept:%f,stdError:%f,correlation:%f},", slope, intercept, stdError, correlation)
	jsonStr += "}"

	return jsonStr
}

func parseLine(coords string) (p Point, err error) {
	if len(coords) > 0 {
		coordsAr := strings.SplitN(strings.TrimSpace(coords), ",", 3)
		if len(coordsAr) > 1 {
			// ignore conversion errors
			p.x, err = strconv.ParseFloat(coordsAr[0], 64)
			if err == nil {
				p.y, err = strconv.ParseFloat(coordsAr[1], 64)
			}
		}
	} else {
		err = errors.New("parseLine: No data")
	}
	return p, err
}

// perform linear regression on the data series
// based on Numerical Methods for Engineers, 2nd ed. by Chapra & Canal
func linearRegression(series []Point) (slope float64, intercept float64, stdError float64, correlation float64) {
	len := len(series)
	flen := float64(len) // convenience
	sumx := 0.0
	sumy := 0.0
	sumxy := 0.0
	sumx2 := 0.0
	for ix := 0; ix < len; ix++ {
		x := Point(series[ix]).x
		y := Point(series[ix]).y
		sumx += x
		sumy += y
		sumxy += x * y
		sumx2 += x * x
	}
	xmean := sumx / flen
	ymean := sumy / flen
	slope = (flen*sumxy - sumx*sumy) / (flen*sumx2 - sumx*sumx)
	intercept = ymean - slope*xmean

	st := 0.0
	sr := 0.0
	for ix := 0; ix < len; ix++ {
		x := Point(series[ix]).x
		y := Point(series[ix]).y
		st += (y - ymean) * (y - ymean)
		// guessing the compiler sees this is constant & does sth faster than exponentiation
		sr += (y - (slope*x - intercept)) * (y - (slope*x - intercept))
	}
	stdError = (math.Sqrt((sr / (flen - 2.0)))) // todo: must check that min 2 points are supplied
	correlation = (math.Sqrt(((st - sr) / st)))
	return slope, intercept, stdError, correlation
}
