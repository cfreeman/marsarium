/*
 * Copyright (c) Clinton Freeman 2016
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy of this software and
 * associated documentation files (the "Software"), to deal in the Software without restriction,
 * including without limitation the rights to use, copy, modify, merge, publish, distribute,
 * sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all copies or
 * substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT
 * NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
 * NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM,
 * DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 */
package main

import (
	"fmt"
	"github.com/kidoman/embd"
	"github.com/kidoman/embd/controller/hd44780"
	"github.com/kidoman/embd/controller/pcal9535a"
	_ "github.com/kidoman/embd/host/all"
	"github.com/kidoman/embd/interface/display/characterdisplay"
	"github.com/kidoman/embd/sensor/bme280"
	"time"
)

const (
	AIR_RELAY    = 0
	VACUUM_RELAY = 3
	FLUSH_TIME   = 39
	RELAY_DELAY  = 10

	CO2_RELAY = 1
	CO2_MIX   = 0.96

	N2_RELAY = 2
	N2_MIX   = 0.02

	Ar_RELAY = 3
	Ar_MIX   = 0.02

	BUTTON_PIN   = 14
	BUTTON_RELAY = 2
)

type Marsarium struct {
	Sensor    *bme280.BME280            // Combination pressure, humidity and temperature sensor.
	GasRelays *pcal9535a.PCAL9535A      // Relays for the gas solinoids.
	AuxRelays *pcal9535a.PCAL9535A      // Relays for auxiliary functions: lights and vacuum.
	Button    embd.DigitalPin           // Marsify button input.
	Display   *characterdisplay.Display // What to display.
}

type stateFn func(m Marsarium) (sF stateFn, mNew Marsarium)

func idle(m Marsarium) (sF stateFn, mNew Marsarium) {
	o, err := m.Button.Read()
	if err != nil {
		panic(fmt.Sprintf("%s\n", err))
	}

	// Marsify button has been pressed - enter countdown.
	if o == 0 {
		return countdown, m
	}

	// No button press, remain idle.
	return idle, m
}

func countdown(m Marsarium) (sF stateFn, mNew Marsarium) {
	ticker := time.NewTicker(time.Second)
	timeChan := time.NewTimer(time.Second * 5).C
	tickChan := ticker.C
	countdown := 5
	done := false

	m.Display.Clear()
	m.Display.SetCursor(0, 0)
	updateDisplay(m, "Godspeed little fern")

	m.Display.SetCursor(0, 3)
	updateDisplay(m, fmt.Sprintf("Marsification in %d", countdown))
	countdown = countdown - 1

	for !done {
		select {
		case <-timeChan:
			ticker.Stop()
			done = true
			break
		case <-tickChan:
			m.Display.SetCursor(17, 3)
			updateDisplay(m, fmt.Sprintf("%d", countdown))

			countdown = countdown - 1
		}
	}

	// Countdown completed. Start marsifying.
	return marsify, m
}

func marsify(m Marsarium) (sF stateFn, mNew Marsarium) {
	m.Display.Clear()
	m.Display.SetCursor(0, 0)
	updateDisplay(m, "**MARSIFYING**")

	ticker := time.NewTicker(time.Second * 1)
	stop := make(chan bool, 1)

	go func() {
		for {
			select {
			case <-ticker.C:
				// Blink the Marsify button.
				state := m.AuxRelays.GetPin(BUTTON_RELAY)
				m.AuxRelays.SetPin(BUTTON_RELAY, !state)
			case <-stop:
				return
			}
		}
	}()

	// Warm up sensor.
	readPressure(m.Sensor)

	// Work out current atmospheric pressure.
	atmo := 0.0
	for i := 0; i < 10; i++ {
		atmo += readPressure(m.Sensor)
	}
	atmo = atmo / 10.0

	m.AuxRelays.SetPin(VACUUM_RELAY, true)
	m.GasRelays.SetPin(AIR_RELAY, true)
	m.GasRelays.SetPin(CO2_RELAY, true)
	time.Sleep(FLUSH_TIME * time.Second)
	m.GasRelays.SetPin(CO2_RELAY, false)

	for (atmo * CO2_MIX) < readPressure(m.Sensor) {
		// Leave valve open.
	}
	m.GasRelays.SetPin(AIR_RELAY, false)
	m.AuxRelays.SetPin(VACUUM_RELAY, false)

	m.GasRelays.SetPin(N2_RELAY, true)
	for readPressure(m.Sensor) < (atmo * (CO2_MIX + N2_MIX)) {
		// Leave valve open.
	}
	m.GasRelays.SetPin(N2_RELAY, false)

	m.GasRelays.SetPin(Ar_RELAY, true)
	for readPressure(m.Sensor) < atmo {
		// Leave valve open
	}
	m.GasRelays.SetPin(Ar_RELAY, false)
	stop <- true

	m.Display.Clear()
	m.Display.SetCursor(0, 0)
	updateDisplay(m, "Welcome to Mars.")

	m.Display.SetCursor(0, 2)
	updateDisplay(m, "Current Weather:")

	return monitor, m
}

func monitor(m Marsarium) (sF stateFn, mNew Marsarium) {
	m.AuxRelays.SetPin(BUTTON_RELAY, true)

	v, err := m.Sensor.Measurements()
	if err == nil {
		t := m.Sensor.Temperature(v)
		h := m.Sensor.Humidity(v)
		p := m.Sensor.Pressure(v)

		m.Display.SetCursor(0, 3)
		updateDisplay(m, fmt.Sprintf("%2.0fC, %2.0f", t, h)+"%RH, "+fmt.Sprintf("%3.0fhPa", (p/100.0)))
	}
	time.Sleep(1 * time.Second)

	return monitor, m
}

func updateDisplay(m Marsarium, s string) {
	err := m.Display.Message(s)
	if err != nil {
		panic(fmt.Sprintf("%s\n", err))
	}
}

func readPressure(s *bme280.BME280) float64 {
	m, err := s.Measurements()
	if err != nil {
		panic(fmt.Sprintf("%s\n", err))
	}

	return s.Pressure(m)
}

func main() {
	fmt.Println("MARSARIUM BOOT")

	embd.InitGPIO()
	defer embd.CloseGPIO()

	button, _ := embd.NewDigitalPin(BUTTON_PIN)
	button.SetDirection(embd.In)

	err := embd.InitI2C()
	if err != nil {
		panic(fmt.Sprintf("%s\n", err))
	}
	defer embd.CloseI2C()
	bus := embd.NewI2CBus(1)

	dc, err := hd44780.NewI2C(bus, 0x25, hd44780.PCF8574PinMap, hd44780.RowAddress20Col)
	hd44780.TwoLine(dc)
	if err != nil {
		panic(fmt.Sprintf("%s\n", err))
	}
	dc.Connection.BacklightOn()
	d := characterdisplay.New(dc, 20, 4)

	rGas, err := pcal9535a.New(bus, 0x27)
	if err != nil {
		panic(fmt.Sprintf("%s\n", err))
	}

	rAux, err := pcal9535a.New(bus, 0x26)
	if err != nil {
		panic(fmt.Sprintf("%s\n", err))
	}

	s, err := bme280.New(bus, 0x77)
	if err != nil {
		panic(fmt.Sprintf("%s\n", err))
	}

	marsarium := Marsarium{s, rGas, rAux, button, d}
	update := idle

	d.Clear()
	d.SetCursor(0, 0)
	updateDisplay(marsarium, "Marsarium 9")

	for {
		update, marsarium = update(marsarium)
	}
}
