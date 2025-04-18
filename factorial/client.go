package factorial

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"golang.org/x/net/publicsuffix"
)

const BaseUrl = "https://api.factorialhr.com"

func handleError(spinner *spinner.Spinner, err error) {
	if err != nil {
		spinner.Stop()
		log.Fatal(err)
	}
}

func NewFactorialClient(email, password string, year, month int, in, out string, todayOnly, untilToday bool) *factorialClient {
	spin := spinner.New(spinner.CharSets[14], 60*time.Millisecond)
	spin.Start()
	c := new(factorialClient)
	c.year = year
	c.month = month
	c.clockIn = in
	c.clockOut = out
	c.todayOnly = todayOnly
	c.untilToday = untilToday
	options := cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	}
	jar, _ := cookiejar.New(&options)
	c.Client = http.Client{Jar: jar}
	spin.Suffix = " Logging in..."
	handleError(spin, c.login(email, password))
	spin.Suffix = " Getting periods data..."
	handleError(spin, c.setPeriodId())
	spin.Suffix = " Getting calendar data..."
	handleError(spin, c.setCalendar())
	spin.Suffix = " Getting shifts data..."
	handleError(spin, c.setShifts())
	spin.Stop()
	return c
}

func (c *factorialClient) ClockIn(dryRun bool) {
	spin := spinner.New(spinner.CharSets[14], 60*time.Millisecond)
	var t time.Time
	var message string
	var body []byte
	var entity newShift
	var resp *http.Response
	var ok bool
	now := time.Now()
	//shift.Period_id = int64(c.period_id)
	entity.ClockIn = c.clockIn
	entity.ClockOut = c.clockOut
	entity.Minutes = nil
	entity.EmployeeId, _ = strconv.Atoi(os.Getenv("USERID"))
	entity.Workable = true
	entity.Source = "desktop"
	for _, d := range c.calendar {
		spin.Restart()
		spin.Reverse()
		t = time.Date(c.year, time.Month(c.month), d.Day, 0, 0, 0, 0, time.UTC)
		message = fmt.Sprintf("%s... ", t.Format("02 Jan"))
		spin.Prefix = message + " "
		clockedIn, clockedTimes := c.clockedIn(d.Day, entity)
		if clockedIn {
			message = fmt.Sprintf("%s ❌ Period overlap: %s\n", message, clockedTimes)
		} else if d.IsLeave {
			message = fmt.Sprintf("%s ❌ %s\n", message, d.LeaveName)
		} else if !d.IsLaborable {
			message = fmt.Sprintf("%s ❌ %s\n", message, t.Format("Monday"))
		} else if c.todayOnly && d.Day != now.Day() {
			message = fmt.Sprintf("%s ❌ %s\n", message, "Skipping: --today")
		} else if c.untilToday && d.Day > now.Day() {
			message = fmt.Sprintf("%s ❌ %s\n", message, "Skipping: --until-today")
		} else {
			ok = true
			if !dryRun {
				ok = false
				fmt.Println(d.Date)
				entity.Day = d.Day
				entity.LocationType = "work_from_home"
				entity.Source = "desktop"
				date, err := time.Parse("2006-01-02", d.Date)
				if err != nil {
					fmt.Println("Error al convertir la cadena a date:", err)
					return
				}
				if d.MinutesLeft == 420 {
					//date.Weekday() == time.Weekday(5) || date.Month() == time.Month(7) {
					entity.ClockIn = "08:00"
					entity.ClockOut = "15:00"
					c.clockIn = "08:00"
					c.clockOut = "15:00"
					entity.Date = date.Format("2006-01-02")
					entity.LocationType = "work_from_home"
					entity.Minutes = nil
					entity.ReferenceDate = date.Format("2006-01-02")
					entity.Source = "desktop"
					entity.TimeSettingsBreakConfigurationId = nil
					entity.Workable = true

					body, _ = json.Marshal(entity)
					resp, _ = c.Post(BaseUrl+"/attendance/shifts", "application/json;charset=UTF-8", bytes.NewReader(body))
					if resp.StatusCode == 201 {
						ok = true
					}
					fmt.Println(resp.StatusCode)

				}
				if d.MinutesLeft == 495 {
					ok = c.shiftWithBreak(entity, body, resp, date)
				}
			}
			if ok {
				message = fmt.Sprintf("%s ✅ %s - %s\n", message, c.clockIn, c.clockOut)
			} else {
				message = fmt.Sprintf("%s ❌ Error when attempting to clock in\n", message)
			}
		}
		spin.Stop()
		fmt.Print(message)
	}
	fmt.Println("done!")
}

func (c *factorialClient) shiftWithBreak(entity newShift, body []byte, resp *http.Response, date time.Time) (ok bool) {
	var shiftIn breakShift
	var shiftOut breakShiftOut
	ok = false
	shiftIn.LocationType = entity.LocationType
	shiftIn.EmployeeId = entity.EmployeeId
	shiftIn.Now = date.Format("2006-01-02") + "T09:00"
	body, _ = json.Marshal(shiftIn)
	resp, _ = c.Post(BaseUrl+"/api/v2/resources/attendance/shifts/clock_in", "application/json;charset=UTF-8", bytes.NewBuffer(body))
	ok = resp.StatusCode == 200
	shiftOut.EmployeeId = entity.EmployeeId
	shiftOut.Now = date.Format("2006-01-02") + "T14:15"
	body, _ = json.Marshal(shiftOut)
	resp, _ = c.Post(BaseUrl+"/api/v2/resources/attendance/shifts/break_start", "application/json;charset=UTF-8", bytes.NewBuffer(body))
	ok = resp.StatusCode == 200
	shiftOut.Now = date.Format("2006-01-02") + "T15:00"
	body, _ = json.Marshal(shiftOut)
	resp, _ = c.Post(BaseUrl+"/api/v2/resources/attendance/shifts/break_end", "application/json;charset=UTF-8", bytes.NewBuffer(body))
	ok = resp.StatusCode == 200
	shiftOut.Now = date.Format("2006-01-02") + "T18:00"
	body, _ = json.Marshal(shiftOut)
	resp, _ = c.Post(BaseUrl+"/api/v2/resources/attendance/shifts/clock_out", "application/json;charset=UTF-8", bytes.NewBuffer(body))
	ok = resp.StatusCode == 200
	return ok
}

func (c *factorialClient) login(email, password string) error {
	getCSRFToken := func(resp *http.Response) string {
		data, _ := io.ReadAll(resp.Body)
		err := resp.Body.Close()
		if err != nil {
			return "Login error"
		}
		start := strings.Index(string(data), "<meta name=\"csrf-token\" content=\"") + 33
		end := strings.Index(string(data)[start:], "\" />")
		return string(data)[start : start+end]
	}

	getLoginError := func(resp *http.Response) string {
		data, _ := io.ReadAll(resp.Body)
		err := resp.Body.Close()
		if err != nil {
			return "Login error"
		}
		start := strings.Index(string(data), "<div class=\"flash flash--wrong\">") + 32
		if start < 0 {
			return ""
		}
		end := strings.Index(string(data)[start:], "</div>")
		if start < 0 || end-start > 100 {
			return ""
		}
		return string(data)[start : start+end]
	}

	resp, _ := c.Get(BaseUrl + "/users/sign_in")
	csrfToken := getCSRFToken(resp)
	body := url.Values{
		"authenticity_token": {csrfToken},
		"return_host":        {"factorialhr.es"},
		"user[email]":        {email},
		"user[password]":     {password},
		"user[remember_me]":  {"0"},
		"commit":             {"Sign in"},
	}
	resp, _ = c.PostForm(BaseUrl+"/users/sign_in", body)
	if err := getLoginError(resp); err != "" {
		return errors.New(err)
	}
	return nil
}

func (c *factorialClient) setPeriodId() error {
	err := errors.New("Could not find the specified year/month in the available periods (" + strconv.Itoa(c.month) + "/" + strconv.Itoa(c.year) + ")")
	resp, _ := c.Get(BaseUrl + "/attendance/periods?year=" + strconv.Itoa(c.year) + "&month=" + strconv.Itoa(c.month))
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return err
	}
	var periods []period
	body, _ := io.ReadAll(resp.Body)
	err = json.Unmarshal(body, &periods)
	if err != nil {
		return err
	}
	for _, p := range periods {
		if p.Year == c.year && p.Month == c.month {
			c.employeeId = p.EmployeeId
			c.periodId = p.Id
			return nil
		}
	}
	return err
}

func (c *factorialClient) CheckHourCalendar(calendar []calendarDay) error {
	u, _ := url.Parse(BaseUrl + "/attendance/periods")
	q := u.Query()
	fmt.Print(c.calendar)
	q.Set("year", strconv.Itoa(c.year))
	q.Set("month", strconv.Itoa(c.month))
	q.Set("employee_id", strconv.Itoa(c.employeeId))
	q.Set("start_on", c.calendar[0].Date)
	q.Set("end_on", c.calendar[len(c.calendar)-1].Date)
	u.RawQuery = q.Encode()
	resp, _ := c.Get(u.String())
	if resp.StatusCode != 200 {
		return errors.New("Error retrieving calendar data")
	}
	defer resp.Body.Close()
	var minutesLeft []Period
	body, _ := io.ReadAll(resp.Body)
	err := json.Unmarshal(body, &minutesLeft)
	if err != nil {
		return err
	}
	for i, _ := range c.calendar {
		c.calendar[i].MinutesLeft = minutesLeft[0].EstimatedRegularMinutesDistribution[i]
	}
	return nil
}

func (c *factorialClient) setCalendar() error {
	u, _ := url.Parse(BaseUrl + "/attendance/calendar")
	q := u.Query()
	q.Set("id", strconv.Itoa(c.employeeId))
	q.Set("year", strconv.Itoa(c.year))
	q.Set("month", strconv.Itoa(c.month))
	u.RawQuery = q.Encode()
	resp, _ := c.Get(u.String())
	if resp.StatusCode != 200 {
		return errors.New("Error retrieving calendar data")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	err := json.Unmarshal(body, &c.calendar)
	if err != nil {
		return err
	}
	sort.Slice(c.calendar, func(i, j int) bool {
		return c.calendar[i].Day < c.calendar[j].Day
	})
	err = c.CheckHourCalendar(c.calendar)
	if err != nil {
		return err
	}

	return nil
}

func (c *factorialClient) setShifts() error {
	u, _ := url.Parse(BaseUrl + "/attendance/shifts")
	q := u.Query()
	q.Set("employee_id", strconv.Itoa(c.employeeId))
	q.Set("year", strconv.Itoa(c.year))
	q.Set("month", strconv.Itoa(c.month))
	u.RawQuery = q.Encode()
	resp, _ := c.Get(u.String())
	if resp.StatusCode != 200 {
		return errors.New("Error retrieving shifts data")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	err := json.Unmarshal(body, &c.shifts)
	if err != nil {
		return err
	}
	return nil
}

func (c *factorialClient) clockedIn(day int, inputShift newShift) (bool, string) {
	clockIn, _ := strconv.Atoi(strings.Join(strings.Split(inputShift.ClockIn, ":"), ""))
	clockOut, _ := strconv.Atoi(strings.Join(strings.Split(inputShift.ClockOut, ":"), ""))
	for _, shift := range c.shifts {
		if shift.Day == day {
			shiftClockIn, _ := strconv.Atoi(strings.Join(strings.Split(shift.ClockIn, ":"), ""))
			shiftClockOut, _ := strconv.Atoi(strings.Join(strings.Split(shift.ClockOut, ":"), ""))
			if (clockIn < shiftClockIn && shiftClockIn < clockOut) ||
				(clockIn < shiftClockOut && shiftClockOut < clockOut) ||
				(shiftClockIn <= clockIn && shiftClockOut >= clockOut) {
				return true, strings.Join([]string{shift.ClockIn, shift.ClockOut}, " - ")
			}
		}
	}
	return false, ""
}

func (c *factorialClient) ResetMonth() {
	var t time.Time
	var message string
	for _, shift := range c.shifts {
		req, _ := http.NewRequest("DELETE", BaseUrl+"/attendance/shifts/"+strconv.Itoa(int(shift.Id)), nil)
		resp, _ := c.Do(req)
		t = time.Date(c.year, time.Month(c.month), shift.Day, 0, 0, 0, 0, time.UTC)
		message = fmt.Sprintf("%s... ", t.Format("02 Jan"))
		if resp.StatusCode != 204 {
			fmt.Print(fmt.Sprintf("%s ❌ Error when attempting to delete shift: %s - %s\n", message, shift.ClockIn, shift.ClockOut))
		} else {
			fmt.Print(fmt.Sprintf("%s ✅ Shift deleted: %s - %s\n", message, shift.ClockIn, shift.ClockOut))
		}
		defer resp.Body.Close()
	}
	fmt.Println("done!")
}
