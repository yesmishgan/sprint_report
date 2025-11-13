package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/olekukonko/tablewriter"
)

// MemberConfig ...
type MemberConfig struct {
	Login    string  `json:"login"`
	Capacity float64 `json:"capacity"`
}

// TeamConfig ...
type TeamConfig struct {
	Team    string         `json:"team"`
	Members []MemberConfig `json:"members"`
}

// MemberState ...
type MemberState struct {
	Login    string  `json:"login"`
	Capacity float64 `json:"capacity"`
	Planned  float64 `json:"planned"`
}

// IssueBaseline ...
type IssueBaseline struct {
	Key string  `json:"key"`
	SP  float64 `json:"sp"`
}

const (
	fieldAssignee      = "Assignee"
	fieldIssueKey      = "Issue key"
	fieldStoryPoints   = "Custom field (Story Points)"
	fieldResponsibleQA = "Custom field (Responsible QA)"
	fieldQAEstimate    = "Custom field (QA Estimate)"
	fieldStatus        = "Status"
)

// Ticket ...
type Ticket map[string]string

// GetAssignee ...
func (t Ticket) GetAssignee() string {
	return strings.TrimSpace(t[fieldAssignee])
}

// GetIssueKey ...
func (t Ticket) GetIssueKey() string {
	return strings.TrimSpace(t[fieldIssueKey])
}

// GetResponsibleQA ...
func (t Ticket) GetResponsibleQA() string {
	return strings.TrimSpace(t[fieldResponsibleQA])
}

// GetStoryPoints ...
func (t Ticket) GetStoryPoints() float64 {
	return parseFloat(t[fieldStoryPoints])
}

// GetQAEstimate ...
func (t Ticket) GetQAEstimate() float64 {
	return parseFloat(t[fieldQAEstimate])
}

// GetStatus ...
func (t Ticket) GetStatus() string {
	return strings.TrimSpace(t[fieldStatus])
}

// Row ...
type Row struct {
	Name      string  `json:"name"`
	Capacity  float64 `json:"capacity"`
	Planned   float64 `json:"planned"`
	Completed float64 `json:"completed"`
	Delivery  float64 `json:"delivery"`
}

// SprintState ...
type SprintState struct {
	SprintName string          `json:"sprint_name,omitempty"` // опционально, ни на что не влияет
	Team       string          `json:"team"`
	Members    []MemberState   `json:"members"`
	Issues     []IssueBaseline `json:"issues"`
}

func main() {
	cmd := flag.String("cmd", "", "Command: init or report")
	csvPath := flag.String("csv", "", "Path to CSV export")
	statePath := flag.String("state", "sprint_state.json", "Path to sprint state JSON")

	// init
	configPath := flag.String("config", "", "Team config JSON (for init)")
	sprintName := flag.String("sprint", "", "Optional sprint name for reference (not used for filtering)")

	// report
	doneStatusesStr := flag.String("done-statuses", "Done,Closed,Resolved", "Comma-separated list of done statuses (for report)")
	format := flag.String("format", "table", "Output format for report: table or json")

	flag.Parse()

	switch *cmd {
	case "init":
		if *csvPath == "" || *configPath == "" {
			exitErr("init requires -csv and -config")
		}
		if err := runInit(*csvPath, *configPath, *statePath, *sprintName); err != nil {
			exitErr(err.Error())
		}
	case "report":
		if *csvPath == "" || *statePath == "" {
			exitErr("report requires -csv and -state")
		}
		if err := runReport(*csvPath, *statePath, *doneStatusesStr, *format); err != nil {
			exitErr(err.Error())
		}
	default:
		exitErr("unknown or empty -cmd (use init or report)")
	}
}

func runInit(csvPath, configPath, statePath, sprintName string) error {
	conf, err := loadConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	records, err := readCSV(csvPath)
	if err != nil {
		return fmt.Errorf("read csv: %w", err)
	}

	plannedByUser, baselines := computePlannedAndBaselines(records)

	state := SprintState{
		SprintName: sprintName,
		Team:       conf.Team,
	}

	for _, mc := range conf.Members {
		taskPlanned := plannedByUser[mc.Login]
		ms := MemberState{
			Login:    mc.Login,
			Capacity: mc.Capacity,
			Planned:  toFixed(taskPlanned, 2),
		}
		state.Members = append(state.Members, ms)
	}

	state.Issues = baselines

	if err := saveState(statePath, &state); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	fmt.Printf("Saved sprint state to %s\n", statePath)
	return nil
}

// computePlannedAndBaselines:
//
//   - считает planned SP по стартовой выгрузке по всем пользователям;
//   - фиксирует baseline задач (key + original_sp) для всех задач со SP > 0.
//     Нет фильтра по конфигу и по имени спринта: считаем, что выгрузка — это весь спринт.
func computePlannedAndBaselines(records []Ticket) (map[string]float64, []IssueBaseline) {
	planned := make(map[string]float64)
	baselineMap := make(map[string]float64)

	for _, row := range records {
		// скипаем непривязанные тикеты
		assignee := row.GetAssignee()
		if assignee == "" {
			continue
		}

		// получаем сторипоинты задачи
		sp := row.GetStoryPoints()
		if sp <= 0 {
			continue
		}

		planned[assignee] += sp

		// добавляем запланированные QA Estimate для тестировщика
		qa := row.GetResponsibleQA()
		qaSp := row.GetQAEstimate()
		planned[qa] += qaSp

		key := row.GetIssueKey()
		if key != "" {
			// фиксируем только первый зафиксированный SP как baseline
			if _, exists := baselineMap[key]; !exists {
				baselineMap[key] = sp
			}
		}
	}

	var baselines []IssueBaseline
	for k, v := range baselineMap {
		baselines = append(baselines, IssueBaseline{
			Key: k,
			SP:  v,
		})
	}

	return planned, baselines
}

func runReport(csvPath, statePath, doneStatuses, format string) error {
	state, err := loadState(statePath)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	records, err := readCSV(csvPath)
	if err != nil {
		return fmt.Errorf("read csv: %w", err)
	}

	doneList := splitAndTrim(doneStatuses)
	doneSet := make(map[string]struct{})
	for _, s := range doneList {
		doneSet[strings.ToLower(s)] = struct{}{}
	}

	// baseline по задачам
	baseline := make(map[string]float64)
	for _, ib := range state.Issues {
		baseline[ib.Key] = ib.SP
	}

	// накапливаем по всем логинам (без фильтра по конфигу)
	completedSP := make(map[string]float64) // SP по done задачам (Assignee)
	completedQA := make(map[string]float64) // QA Estimate по done задачам (Responsible QA)
	partialDone := make(map[string]float64) // delta по baseline незавершённым задачам

	for _, row := range records {
		status := strings.ToLower(row.GetStatus())
		_, isDone := doneSet[status]

		var (
			key      = row.GetIssueKey()
			assignee = row.GetAssignee()
			qa       = row.GetResponsibleQA()
		)

		spNow := row.GetStoryPoints()
		qaSp := row.GetQAEstimate()
		origSP, isBaseline := baseline[key]

		if isDone {
			// Полностью завершённые задачи

			// 1) Story points исполнителю
			if assignee != "" && spNow > 0 {
				completedSP[assignee] += spNow
			}

			// 2) QA Estimate ответственным QA
			if qaSp > 0 && qa != "" {
				completedQA[qa] += qaSp
			}

			continue
		}

		// Не done: частичное выполнение baseline-задач
		if isBaseline && origSP > 0 && spNow >= 0 {
			delta := origSP - spNow
			if delta > 0 && assignee != "" {
				partialDone[assignee] += delta
			}
		}
	}

	var rows []Row
	var teamCap, teamPlanned, teamCompleted float64

	for _, m := range state.Members {
		login := m.Login

		totalCompleted :=
			completedSP[login] +
				completedQA[login] +
				partialDone[login]

		delivery := 0.0
		if m.Planned > 0 {
			delivery = (totalCompleted / m.Planned) * 100.0
		}

		row := Row{
			Name:      login,
			Capacity:  m.Capacity,
			Planned:   m.Planned,
			Completed: totalCompleted,
			Delivery:  delivery,
		}
		rows = append(rows, row)

		teamCap += m.Capacity
		teamPlanned += m.Planned
		teamCompleted += totalCompleted
	}

	teamDelivery := 0.0
	if teamPlanned > 0 {
		teamDelivery = (teamCompleted / teamPlanned) * 100.0
	}

	teamRow := Row{
		Name:      fmt.Sprintf("Team (%s)", state.Team),
		Capacity:  teamCap,
		Planned:   teamPlanned,
		Completed: teamCompleted,
		Delivery:  teamDelivery,
	}

	if strings.ToLower(format) == "json" {
		out := struct {
			SprintName string `json:"sprint_name,omitempty"`
			Team       Row    `json:"team"`
			Users      []Row  `json:"users"`
		}{
			SprintName: state.SprintName,
			Team:       teamRow,
			Users:      rows,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	printTable(teamRow, rows)
	return nil
}

// ---------- IO helpers ----------

func exitErr(msg string) {
	fmt.Fprintln(os.Stderr, "Error:", msg)
	os.Exit(1)
}

func loadConfig(path string) (*TeamConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var c TeamConfig
	if err := json.NewDecoder(f).Decode(&c); err != nil {
		return nil, err
	}
	return &c, nil
}

func saveState(path string, state *SprintState) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(state)
}

func loadState(path string) (*SprintState, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var s SprintState
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

func readCSV(path string) ([]Ticket, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.TrimLeadingSpace = true

	header, err := r.Read()
	if err != nil {
		return nil, err
	}

	var rows []Ticket
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		row := make(map[string]string, len(header))
		for i, h := range header {
			if i < len(rec) {
				row[h] = rec[i]
			} else {
				row[h] = ""
			}
		}
		rows = append(rows, row)
	}

	return rows, nil
}

// ---------- calc helpers ----------

func round(num float64) int {
	return int(num + math.Copysign(0.5, num))
}

func toFixed(num float64, precision int) float64 {
	output := math.Pow(10, float64(precision))
	return float64(round(num*output)) / output
}

func parseFloat(s string) float64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return toFixed(f, 2)
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	var res []string
	for _, p := range parts {
		pp := strings.TrimSpace(p)
		if pp != "" {
			res = append(res, pp)
		}
	}
	return res
}

// ---------- output ----------

func printTable(team Row, rows []Row) {
	table := tablewriter.NewTable(os.Stdout)
	table.Header([]string{"Name", "Capacity", "Planned", "Completed", "Delivery"})

	teamRow := []string{
		team.Name,
		fmt.Sprintf("%.2f", team.Capacity),
		fmt.Sprintf("%.2f", team.Planned),
		fmt.Sprintf("%.2f", team.Completed),
		fmt.Sprintf("%.0f%%", team.Delivery),
	}
	table.Append(teamRow)

	for _, r := range rows {
		table.Append([]string{
			r.Name,
			fmt.Sprintf("%.2f", r.Capacity),
			fmt.Sprintf("%.2f", r.Planned),
			fmt.Sprintf("%.2f", r.Completed),
			fmt.Sprintf("%.0f%%", r.Delivery),
		})
	}
	_ = table.Render()
}
