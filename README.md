# factorialsucks

A command-line tool to automate clock-in/out in FactorialHR.

## Features

- Automatically clocks in/out for the entire month
- Handles breaks automatically
- Supports different schedules:
  - Regular schedule (Monday-Thursday): 9:00-18:00 with breaks
  - Friday schedule: 8:00-15:00 without breaks
  - Days before holidays: 8:00-15:00 without breaks
  - Summer schedule (July 1st - September 15th): 8:00-15:00 without breaks

## Installation

1. Make sure you have Go installed on your system
2. Clone this repository:

```bash
git clone https://github.com/jtrancoso-alt/factorialsucks.git
cd factorialsucks
```

## Configuration

Create a `.env` file in the project root with the following variables:

```env
EMAIL=your.email@company.com
PASSWORD=your_factorial_password
USERID=your_factorial_user_id
```

You can find your user ID in the URL in Factorial -> Employees and your user (e.g., if the URL is `https://app.factorialhr.com/employees/12345`, your user ID is `12345`).

## Usage

```bash
go run factorialsucks.go [options]
```

### Options

```
--email value, -e value        Your Factorial email address
--year YYYY, -y YYYY          Year to manage (default: current year)
--month MM, -m MM             Month to manage (default: current month)
--clock-in HH:MM, --ci HH:MM  Clock-in time (default: "09:00")
--clock-out HH:MM, --co HH:MM Clock-out time (default: "18:00")
--today, -t                   Add shift for today only
--until-today, --ut           Add shifts only until today
--dry-run, --dr              Preview changes without applying them
--reset-month, --rm          Remove all shifts for the given month
--help, -h                   Show help
```

### Examples

1. Add shifts for the whole month:

```bash
go run factorialsucks.go
```

2. Add a shift for today only:

```bash
go run factorialsucks.go --today
```

3. Remove all shifts for a specific month:

```bash
go run factorialsucks.go --reset-month --month 3 --year 2024
```

4. Preview changes without applying them:

```bash
go run factorialsucks.go --dry-run
```

## Schedule Rules

The tool automatically handles different schedules based on the following rules:

1. **Regular Days (Monday-Thursday)**:

   - Clock in: 9:00
   - Break: 14:15 - 15:00
   - Clock out: 18:00

2. **Fridays**:

   - Clock in: 8:00
   - Clock out: 15:00
   - No breaks

3. **Days Before Holidays**:

   - Clock in: 8:00
   - Clock out: 15:00
   - No breaks

4. **Summer Schedule**:

   - Clock in: 8:00
   - Clock out: 15:00
   - No breaks
   - Automatically detected from Factorial planning versions

## Credits

This tool is a fork of the original [factorialsucks](https://github.com/alejoar/factorialsucks) created by [@alejoar](https://github.com/alejoar). The original version has been modified to better handle break times and different schedules for weekdays and Fridays.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
