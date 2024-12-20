package main

import (
	_ "github.com/joho/godotenv/autoload"
	"log"
	"os"
	"time"

	"github.com/alejoar/factorialsucks/factorial"
	"github.com/urfave/cli/v2"
)

var today time.Time = time.Now()

func main() {
	log.SetFlags(0)
	app := &cli.App{
		Name:            "factorialsucks",
		Usage:           "FactorialHR auto clock in for the whole month from the command line",
		Version:         "2.1",
		Compiled:        time.Now(),
		UsageText:       "factorialsucks [options]",
		HideHelpCommand: true,
		HideVersion:     true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "email",
				Aliases: []string{"e"},
				Usage:   "you factorial email address",
			},
			&cli.IntFlag{
				Name:        "year",
				Aliases:     []string{"y"},
				Usage:       "clock-in year `YYYY`",
				DefaultText: "current year",
				Value:       today.Year(),
			},
			&cli.IntFlag{
				Name:        "month",
				Aliases:     []string{"m"},
				Usage:       "clock-in month `MM`",
				DefaultText: "current month",
				Value:       int(today.Month()),
			},
			&cli.StringFlag{
				Name:    "clock-in",
				Aliases: []string{"ci"},
				Usage:   "clock-in time `HH:MM`",
				Value:   "09:00",
			},
			&cli.StringFlag{
				Name:    "clock-out",
				Aliases: []string{"co"},
				Usage:   "clock-in time `HH:MM`",
				Value:   "18:00",
			},
			&cli.BoolFlag{
				Name:    "today",
				Aliases: []string{"t"},
				Usage:   "clock in for today only",
				Value:   false,
			},
			&cli.BoolFlag{
				Name:    "until-today",
				Aliases: []string{"ut"},
				Usage:   "clock in only until today",
				Value:   false,
			},
			&cli.BoolFlag{
				Name:    "dry-run",
				Aliases: []string{"dr"},
				Usage:   "do a dry run without actually clocking in",
			},
			&cli.BoolFlag{
				Name:    "reset-month",
				Aliases: []string{"rm"},
				Usage:   "delete all shifts for the given month",
				Value:   false,
			},
		},
		Action: factorialSucks,
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func factorialSucks(c *cli.Context) error {
	var year, month int
	//email, password := readCredentials(c)
	email := os.Getenv("EMAIL")
	password := os.Getenv("PASSWORD")
	todayOnly := c.Bool("today")
	if todayOnly {
		year = today.Year()
		month = int(today.Month())
	} else {
		year = c.Int("year")
		month = int(today.Month())
	}
	clockIn := c.String("clock-in")
	clockOut := c.String("clock-out")
	dryRun := c.Bool("dry-run")
	untilToday := c.Bool("until-today")
	resetMonth := c.Bool("reset-month")
	//reset_month = true

	client := factorial.NewFactorialClient(email, password, year, month, clockIn, clockOut, todayOnly, untilToday)
	if resetMonth {
		client.ResetMonth()
	} else {
		client.ClockIn(dryRun)
	}
	return nil
}
