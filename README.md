# ComSSA Sheets

## Setup

1. Add `service-account.json` file and fill out environment variables in .env file (see example.env)
2. Add the service account email to your Google Sheet with editor permissions
3. Edit crontab to run every hour
`0 * * * * /usr/local/go/bin/go run /path/to/main.go >> /path/to/log.txt 2>&1`
