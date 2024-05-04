package main

import (
	"log"
	"math"
	"math/cmplx"
	"os"

	custom_error "rtl_sdr_pulse_detect/error"

	"github.com/pa-m/numgo"

	"github.com/go-cmd/cmd"
	"github.com/tidwall/gjson"
)

var settings_json_string = `
--EXAMPLE--
{
	"rtl_sdr_path" : "rtl_sdr",
	"frequency_hz": 433920000,
	"sample_rate_hz": 1000000,
	"rtl_sdr_rf_gain": 9,
	"buffer_size": 262144,
	"min_pulse_db": 2,
	"output_file_path": "output_file"
}
--EXAMPLE--
`
var np = numgo.NumGo{}
var power_dbm = math.Inf(0)

func main() {
	log.Println("*")
	bytes, err := os.ReadFile("settings.json")
	custom_error.Fatal(err)
	settings_json_string = string(bytes)

	output_file_path := gjson.Get(settings_json_string, "output_file_path").String()
	os.Remove(output_file_path)

	rtl_sdr_path := gjson.Get(settings_json_string, "rtl_sdr_path").String() //From Osmocom (Windows: rtl_sdr.exe)
	frequency_hz := gjson.Get(settings_json_string, "frequency_hz").String()
	sample_rate_hz := gjson.Get(settings_json_string, "sample_rate_hz").String()
	rtl_sdr_rf_gain := gjson.Get(settings_json_string, "rtl_sdr_rf_gain").String()

	command := cmd.NewCmd(rtl_sdr_path, "-f", frequency_hz, "-s", sample_rate_hz, "-g", rtl_sdr_rf_gain, output_file_path)
	command_channel := command.Start()

	for {
		if _, err := os.Stat(output_file_path); err == nil {
			break
		}
	}

	buf_size := gjson.Get(settings_json_string, "buffer_size").Int()
	iteration := 1
	for {
		file_info, err := os.Stat(output_file_path)
		custom_error.Fatal(err)

		if !(file_info.Size() >= buf_size*int64(iteration)) {
			continue
		}

		file, err := os.Open(output_file_path)
		custom_error.Fatal(err)
		defer file.Close()

		buf := make([]byte, buf_size)
		file.ReadAt(buf, (buf_size*int64(iteration))-buf_size)
		if !process_bytes(buf) {
			command.Stop()
			break
		}
		iteration++
	}
	<-command_channel
	log.Println("**")
}

func process_bytes(bytes []byte) bool {
	complexes := make([]complex128, 0)

	//ref IQArray: convert_to (Universal Radio Hacker) uint8 to int8
	for i := 0; i < len(bytes); i += 2 {
		complexes = append(complexes, complex(float64(bytes[i])-128, float64(bytes[i+1])-128))
	}

	magnitudes := make([]float64, 0)
	for _, c := range complexes {
		magnitudes = append(magnitudes, cmplx.Abs(c))
	}

	//ref IQArray: magnitudes_normalized (Universal Radio Hacker)
	normalized_magnitudes := make([]float64, 0)
	for _, m := range magnitudes {
		normalized_magnitudes = append(normalized_magnitudes, m/math.Hypot(math.MaxInt8, math.MinInt8))
	}

	average_magnitude := np.Mean(normalized_magnitudes)

	// ref SignalFrame:update_number_selected_samples (Universal Radio Hacker)
	previous_power_dbm := power_dbm
	if average_magnitude > 0 {
		power_dbm = 10 * math.Log10(average_magnitude)
	} else {
		power_dbm = math.Inf(0)
	}

	if !(math.IsInf(previous_power_dbm, 0) || math.IsInf(power_dbm, 0)) {
		db := 10 * math.Log10(previous_power_dbm/power_dbm)
		if !math.Signbit(db) && db > gjson.Get(settings_json_string, "min_pulse_db").Float() { //power increase
			log.Println("PULSE")
		}
	}

	return true

}
