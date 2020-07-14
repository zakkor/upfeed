package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
)

type Job struct {
	Title    string    `json:"title"`
	PostedOn time.Time `json:"posted_on"`
	Category string    `json:"category"`
	Skills   []string  `json:"skills"`
	Country  string    `json:"country"`

	// If false, job is fixed price.
	IsHourly    bool       `json:"is_hourly"`
	HourlyRange [2]float32 `json:"hourly_range"`
	// Only if IsHourly == false
	Budget int `json:"budget"`
}

func main() {
	var (
		feedURL = flag.String("feed", "", "")
		saveDir = flag.String("saveDir", "", "")
	)
	flag.Parse()

	if *feedURL == "" {
		panic("please specify -feed")
	}
	if *saveDir == "" {
		panic("please specify -saveDir")
	}

	var (
		jobs           = LoadJobs(*saveDir, "filtered")
		jobsUnfiltered = LoadJobs(*saveDir, "unfiltered")
		feedParser     = gofeed.NewParser()
	)

	var recentJob time.Time
	for {
		feed, err := feedParser.ParseURL(*feedURL)
		if err != nil {
			log.Fatalln(err)
		}

		for _, item := range feed.Items {
			job := ParseJob(item)
			jobsUnfiltered[job.PostedOn] = job
			SaveJobs(*saveDir, "unfiltered", jobsUnfiltered)

			if job.PostedOn.After(recentJob) {
				recentJob = job.PostedOn

				if junk, reason := job.Junk(); !junk {
					// New legit job was posted, save and send notification
					jobs[job.PostedOn] = job
					SaveJobs(*saveDir, "filtered", jobs)

					err = Notify(job.Title, job.Format(), "assets/information.png")
					if err != nil {
						log.Fatalln(err)
					}
				} else {
					// Job is junk
					err = Notify("Job filtered out", reason, "assets/information.png")
					if err != nil {
						log.Fatalln(err)
					}
				}
			}
		}

		time.Sleep(30 * time.Second)
	}
}

func ParseJob(item *gofeed.Item) Job {
	re := regexp.MustCompile(`<b>([a-zA-Z ]+)<\/b>:(.[^<]+)<`)

	var job Job
	// Hourly by default
	job.IsHourly = true

	job.Title = strings.TrimRight(item.Title, " - Upwork")

	matches := re.FindAllStringSubmatch(item.Content, -1)
	for _, match := range matches {
		key := strings.TrimSpace(match[1])
		val := strings.TrimSpace(match[2])

		switch key {
		case "Posted On":
			layout := "January 2, 2006 15:04 MST"
			t, err := time.Parse(layout, val)
			if err != nil {
				log.Fatal(err)
			}
			job.PostedOn = t
		case "Category":
			job.Category = val
		case "Skills":
			skills := strings.Split(val, ", ")
			for i := range skills {
				skills[i] = strings.TrimSpace(skills[i])
			}
			job.Skills = skills
		case "Country":
			job.Country = val
		case "Budget":
			val = strings.ReplaceAll(val, "$", "")
			val = strings.ReplaceAll(val, ",", "")
			budget64, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				log.Fatalln(err)
			}
			job.Budget = int(budget64)
			job.IsHourly = false
		case "Hourly Range":
			val = strings.ReplaceAll(val, "$", "")
			split := strings.Split(val, "-")
			job.HourlyRange = [2]float32{0.0, 0.0}
			lower64, err := strconv.ParseFloat(split[0], 32)
			if err != nil {
				log.Fatalln(err)
			}
			job.HourlyRange[0] = float32(lower64)
			if len(split) > 1 {
				upper64, err := strconv.ParseFloat(split[1], 32)
				if err != nil {
					log.Fatalln(err)
				}
				job.HourlyRange[1] = float32(upper64)
			}
		}
	}

	return job
}

func (j *Job) Format() string {
	format := fmt.Sprintf("Country: %s\n", j.Country)

	if j.IsHourly {
		format += fmt.Sprintf("Type: Hourly\nHourly Range: $%v-$%v\n", j.HourlyRange[0], j.HourlyRange[1])
	} else {
		format += fmt.Sprintf("Type: Fixed price\nBudget: $%v\n", j.Budget)
	}

	return format
}

// Junk returns true and the reason for filtering, if the job should be filtered out.
func (j *Job) Junk() (bool, string) {
	if j.Country == "India" || j.Country == "Nigeria" {
		return true, fmt.Sprintf("Country is %s", j.Country)
	}

	return false, ""
}

func SaveJobs(dir, category string, jobs map[time.Time]Job) {
	filename := fmt.Sprintf("upfeed_%s_%s.json", time.Now().Format("02-01-2006"), category)

	var js []Job
	for _, job := range jobs {
		js = append(js, job)
	}

	sort.Slice(js, func(i, j int) bool {
		return js[i].PostedOn.After(js[j].PostedOn)
	})

	data, err := json.Marshal(js)
	if err != nil {
		log.Fatalln(err)
	}

	path := filepath.Join(dir, filename)
	err = ioutil.WriteFile(path, data, 0755)
	if err != nil {
		log.Fatalln(err)
	}
}

func LoadJobs(dir, category string) map[time.Time]Job {
	filename := fmt.Sprintf("upfeed_%s_%s.json", time.Now().Format("02-01-2006"), category)
	path := filepath.Join(dir, filename)
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return make(map[time.Time]Job)
	}

	var js []Job
	err = json.Unmarshal(data, &js)
	if err != nil {
		log.Fatalln(err)
	}

	var jobs = make(map[time.Time]Job)
	for _, job := range js {
		jobs[job.PostedOn] = job
	}

	return jobs
}
