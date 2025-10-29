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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"golang.org/x/net/publicsuffix"
)

const BaseUrl = "https://api.factorialhr.com"

// Shift durations in minutes
const (
	RegularShiftMinutes = 495 // 8:15 hours
	FridayShiftMinutes  = 420 // 7:00 hours
)

// NewFactorialClient creates a new client and initializes it with the required data
func NewFactorialClient(email, password string, year, month int, in, out string, todayOnly, untilToday bool) *factorialClient {
	spin := spinner.New(spinner.CharSets[14], 60*time.Millisecond)
	spin.Start()

	c := &factorialClient{
		year:       year,
		month:      month,
		clockIn:    in,
		clockOut:   out,
		todayOnly:  todayOnly,
		untilToday: untilToday,
	}

	// Setup HTTP client with cookie jar
	options := cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	}
	jar, _ := cookiejar.New(&options)
	c.Client = http.Client{Jar: jar}

	// Initialize client data
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

// ClockIn adds shifts for the specified period
func (c *factorialClient) ClockIn(dryRun bool) {
	spin := spinner.New(spinner.CharSets[14], 60*time.Millisecond)
	now := time.Now()

	for _, day := range c.calendar {
		spin.Restart()
		spin.Reverse()

		date := time.Date(c.year, time.Month(c.month), day.Day, 0, 0, 0, 0, time.UTC)
		message := fmt.Sprintf("%s... ", date.Format("02 Jan"))
		spin.Prefix = message + " "

		// Skip if conditions are not met
		if skip, reason := c.shouldSkipDay(day, date, now); skip {
			message = fmt.Sprintf("%s ❌ %s\n", message, reason)
			spin.Stop()
			fmt.Print(message)
			continue
		}

		// Create and add shift
		shift := c.createShift(day)
		if !dryRun {
			ok := c.addShift(shift)
			if ok {
				message = fmt.Sprintf("%s ✅ %s - %s\n", message, shift.ClockIn, shift.ClockOut)
			} else {
				message = fmt.Sprintf("%s ❌ Error when attempting to clock in\n", message)
			}
		} else {
			message = fmt.Sprintf("%s ✅ %s - %s (dry run)\n", message, shift.ClockIn, shift.ClockOut)
		}

		spin.Stop()
		fmt.Print(message)
	}
	fmt.Println("done!")
}

// shouldSkipDay determines if a day should be skipped and why
func (c *factorialClient) shouldSkipDay(day calendarDay, date time.Time, now time.Time) (bool, string) {
	// Check for existing shifts
	if clockedIn, times := c.clockedIn(day.Day, newShift{ClockIn: c.clockIn, ClockOut: c.clockOut}); clockedIn {
		return true, fmt.Sprintf("Period overlap: %s", times)
	}

	// Check for leaves
	if day.IsLeave {
		return true, day.LeaveName
	}

	// Check for non-laborable days
	if !day.IsLaborable {
		return true, date.Format("Monday")
	}

	// Check for today-only flag
	if c.todayOnly && day.Day != now.Day() {
		return true, "Skipping: --today"
	}

	// Check for until-today flag
	if c.untilToday && day.Day > now.Day() {
		return true, "Skipping: --until-today"
	}

	return false, ""
}

// createShift creates a shift for the given day with appropriate times
func (c *factorialClient) createShift(day calendarDay) newShift {
	shift := newShift{
		ClockIn:                          c.clockIn,
		ClockOut:                         c.clockOut,
		Day:                              day.Day,
		EmployeeId:                       c.employeeId,
		Workable:                         true,
		LocationType:                     "work_from_home",
		Source:                           "desktop",
		TimeSettingsBreakConfigurationId: nil,
		Minutes:                          nil,
	}

	// Parse the date to check schedule
	date, _ := time.Parse("2006-01-02", day.Date)
	month := date.Month()
	dayOfMonth := date.Day()

	// Check if date is in summer schedule (July 1st to September 15th)
	isSummerSchedule := (month == time.July) ||
		(month == time.August) ||
		(month == time.September && dayOfMonth < 15)

	// Use 7-hour schedule (8:00-15:00) for:
	// 1. Fridays (MinutesLeft == FridayShiftMinutes)
	// 2. Days before holidays (DayBeforeHoliday == true)
	// 3. Summer schedule (July 1st to September 15th)
	if day.MinutesLeft == FridayShiftMinutes || day.DayBeforeHoliday || isSummerSchedule {
		shift.ClockIn = "08:00"
		shift.ClockOut = "15:00"
	}

	return shift
}

// addShift adds a shift to Factorial
func (c *factorialClient) addShift(shift newShift) bool {
	// Get calendar day (adjusting for 0-based array index)
	calendarDay := c.calendar[shift.Day-1]
	date, err := time.Parse("2006-01-02", calendarDay.Date)
	if err != nil {
		fmt.Println("Error parsing date:", err)
		return false
	}

	shift.Date = date.Format("2006-01-02")
	shift.ReferenceDate = date.Format("2006-01-02")

	// Parse the date to check for summer schedule
	month := date.Month()
	dayOfMonth := date.Day()

	// Check if date is in summer schedule (July 1st to September 15th)
	isSummerSchedule := (month == time.July) ||
		(month == time.August) ||
		(month == time.September && dayOfMonth < 15)

	// For regular shifts (Monday-Thursday) and not in summer schedule, use breaks
	if calendarDay.MinutesLeft == RegularShiftMinutes && !isSummerSchedule {
		return c.addShiftWithBreak(shift, date)
	}

	// For Friday shifts, days before holidays, and summer schedule, use direct shift without breaks
	body, _ := json.Marshal(shift)
	resp, _ := c.Post(BaseUrl+"/attendance/shifts", "application/json;charset=UTF-8", bytes.NewReader(body))
	return resp.StatusCode == 201
}

// addShiftWithBreak adds a shift with break times
func (c *factorialClient) addShiftWithBreak(shift newShift, date time.Time) bool {
	// Clock in at 9:00
	shiftIn := breakShift{
		EmployeeId:   shift.EmployeeId,
		LocationType: shift.LocationType,
		Now:          date.Format("2006-01-02") + "T08:45",
	}
	if !c.makeBreakRequest(shiftIn, "/clock_in") {
		return false
	}

	// Break start at 14:15
	shiftOut := breakShiftOut{
		EmployeeId: shift.EmployeeId,
		Now:        date.Format("2006-01-02") + "T14:30",
	}
	if !c.makeBreakRequest(shiftOut, "/break_start") {
		return false
	}

	// Break end at 15:00
	shiftOut.Now = date.Format("2006-01-02") + "T15:00"
	if !c.makeBreakRequest(shiftOut, "/break_end") {
		return false
	}

	// Clock out at 18:00
	shiftOut.Now = date.Format("2006-01-02") + "T17:30"
	return c.makeBreakRequest(shiftOut, "/clock_out")
}

// makeBreakRequest makes a request to the break endpoints
func (c *factorialClient) makeBreakRequest(data interface{}, endpoint string) bool {
	body, _ := json.Marshal(data)
	resp, _ := c.Post(BaseUrl+"/api/v2/resources/attendance/shifts"+endpoint, "application/json;charset=UTF-8", bytes.NewBuffer(body))
	if resp.StatusCode != 200 {
		fmt.Printf("Error in %s request: %d\n", endpoint, resp.StatusCode)
		return false
	}
	return true
}

// ResetMonth deletes all shifts for the current month
func (c *factorialClient) ResetMonth() {
	for _, shift := range c.shifts {
		date := time.Date(c.year, time.Month(c.month), shift.Day, 0, 0, 0, 0, time.UTC)
		message := fmt.Sprintf("%s... ", date.Format("02 Jan"))

		req, _ := http.NewRequest("DELETE", BaseUrl+"/attendance/shifts/"+strconv.Itoa(int(shift.Id)), nil)
		resp, _ := c.Do(req)

		if resp.StatusCode != 204 {
			fmt.Print(fmt.Sprintf("%s ❌ Error when attempting to delete shift: %s - %s\n", message, shift.ClockIn, shift.ClockOut))
		} else {
			fmt.Print(fmt.Sprintf("%s ✅ Shift deleted: %s - %s\n", message, shift.ClockIn, shift.ClockOut))
		}
		defer resp.Body.Close()
	}
	fmt.Println("done!")
}

// Helper functions for API calls
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

func handleError(spinner *spinner.Spinner, err error) {
	if err != nil {
		spinner.Stop()
		log.Fatal(err)
	}
}

// CheckHourCalendar retrieves and sets the minutes left for each day in the calendar
func (c *factorialClient) CheckHourCalendar(calendar []calendarDay) error {
	u, _ := url.Parse(BaseUrl + "/attendance/periods")
	q := u.Query()
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
	for i := range c.calendar {
		c.calendar[i].MinutesLeft = minutesLeft[0].EstimatedRegularMinutesDistribution[i]
	}
	return nil
}
