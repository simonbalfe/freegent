package api

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const csvExportPageSize = 1000

type csvExportColumn struct {
	header string
	key    string
}

func handleJobCSV(writer http.ResponseWriter, request *http.Request, store JobBackend) {
	job, err := store.GetPage(request.PathValue("id"), csvExportPageSize, 0)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(writer, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}
	if err != nil {
		writeJSON(writer, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	inputColumns, answerColumn := csvExportColumns(job)
	writer.Header().Set("Content-Type", "text/csv; charset=utf-8")
	writer.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": csvExportFilename(job)}))
	output := csv.NewWriter(writer)
	if err := output.Write(csvExportHeaders(inputColumns, answerColumn)); err != nil {
		return
	}
	offset := 0
	for {
		if offset > 0 {
			job, err = store.GetPage(request.PathValue("id"), csvExportPageSize, offset)
			if err != nil {
				return
			}
		}
		for _, row := range job.Rows {
			if err := output.Write(csvExportRecord(row, inputColumns)); err != nil {
				return
			}
		}
		output.Flush()
		if output.Error() != nil || len(job.Rows) < csvExportPageSize {
			return
		}
		offset += len(job.Rows)
	}
}

func csvExportColumns(job DashboardJob) ([]csvExportColumn, csvExportColumn) {
	inputKeys := map[string]bool{}
	for _, row := range job.Rows {
		for key := range row.Input {
			inputKeys[key] = true
		}
	}
	sortedInputKeys := make([]string, 0, len(inputKeys))
	for key := range inputKeys {
		sortedInputKeys = append(sortedInputKeys, key)
	}
	sort.Strings(sortedInputKeys)
	usedHeaders := map[string]bool{}
	inputColumns := make([]csvExportColumn, 0, len(sortedInputKeys))
	for _, key := range sortedInputKeys {
		header := uniqueCSVHeader(key, usedHeaders)
		inputColumns = append(inputColumns, csvExportColumn{header: header, key: key})
	}
	answerColumn := csvExportColumn{header: uniqueCSVHeader("answer", usedHeaders), key: "answer"}
	return inputColumns, answerColumn
}

func csvExportHeaders(inputColumns []csvExportColumn, answerColumn csvExportColumn) []string {
	headers := make([]string, 0, len(inputColumns)+1)
	for _, column := range inputColumns {
		headers = append(headers, column.header)
	}
	headers = append(headers, answerColumn.header)
	return headers
}

func csvExportRecord(row DashboardRow, inputColumns []csvExportColumn) []string {
	record := make([]string, 0, len(inputColumns)+1)
	for _, column := range inputColumns {
		record = append(record, csvExportCell(row.Input[column.key]))
	}
	answer := any(row.Result.Result)
	if row.Result.Error != "" {
		answer = map[string]any{"error": row.Result.Error}
	}
	record = append(record, csvExportCell(answer))
	return record
}

func csvExportCell(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	switch value.(type) {
	case bool, float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprint(value)
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(encoded)
	}
}

func uniqueCSVHeader(value string, used map[string]bool) string {
	header := strings.TrimSpace(value)
	if header == "" {
		header = "column"
	}
	candidate := header
	for suffix := 2; used[candidate]; suffix++ {
		candidate = header + "_" + strconv.Itoa(suffix)
	}
	used[candidate] = true
	return candidate
}

func csvExportFilename(job DashboardJob) string {
	name := strings.TrimSpace(filepath.Base(job.Name))
	if name == "" || name == "." {
		name = "freegent-" + job.ID
	}
	extension := filepath.Ext(name)
	if extension != "" {
		name = strings.TrimSuffix(name, extension)
	}
	return name + "-results.csv"
}
