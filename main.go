package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
)

type Languages struct {
	Total   int            `json:"total"`
	Stats   map[string]int `json:"stats"`
	Lines   map[string]int `json:"lines"`
	Commits int            `json:"commits"`
	Files   int            `json:"files"`
}

type Plugins struct {
	Languages Languages `json:"languages"`
}

type Meta struct {
	Version   string `json:"version"`
	Generated string `json:"generated"`
}

type Metrics struct {
	Plugins Plugins `json:"plugins"`
	Meta    Meta    `json:"meta"`
}

func cleanFloatString(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', 2, 64)
}

func floatToString(f float64) string {
	if f < 1000 {
		return cleanFloatString(f)
	} else if f < 1000000 {
		return strconv.FormatFloat(f/1000, 'f', 2, 64) + "k"
	} else if f < 1000000000 {
		return strconv.FormatFloat(f/1000000, 'f', 2, 64) + "M"
	}
	return strconv.FormatFloat(f/1000000000, 'f', 2, 64) + "B"
}

func floatToFilesize(f float64, precision int) string {
	const (
		KB = 1000
		MB = 1000 * KB
		GB = 1000 * MB
	)

	switch {
	case f < KB:
		return strconv.FormatFloat(f, 'f', precision, 64) + " b"
	case f < MB:
		return strconv.FormatFloat(f/KB, 'f', precision, 64) + " kb"
	case f < GB:
		return strconv.FormatFloat(f/MB, 'f', precision, 64) + " Mb"
	default:
		return strconv.FormatFloat(f/GB, 'f', precision, 64) + " Gb"
	}
}

type LangData struct {
	Name      string
	Size      float64
	Lines     float64
	Percent   float64
	Emoji     string
	Exact     float64
	Assigned  int
	Remainder float64
}

func GenerateEmojiGrid(input map[string][]float64, rows, cols int) (string, map[string]string) {
	totalSquares := rows * cols

	// Fixed palette
	colors := []string{"🟦", "🟨", "🟥", "🟩", "🟪", "🟧", "🟫", "⬛"}

	// Build and sort languages
	var langs []LangData
	for name, vals := range input {
		langs = append(langs, LangData{
			Name:    name,
			Size:    vals[0],
			Lines:   vals[1],
			Percent: vals[2],
		})
	}
	sort.SliceStable(langs, func(i, j int) bool {
		return langs[i].Percent > langs[j].Percent
	})

	// Assign colors and calculate squares
	for i := range langs {
		langs[i].Emoji = colors[i%len(colors)]
		langs[i].Exact = (langs[i].Percent / 100.0) * float64(totalSquares)
		langs[i].Assigned = int(math.Floor(langs[i].Exact))
		langs[i].Remainder = langs[i].Exact - float64(langs[i].Assigned)
	}

	// Allocate remaining blocks by largest remainder
	assignedTotal := 0
	for _, lang := range langs {
		assignedTotal += lang.Assigned
	}
	remaining := totalSquares - assignedTotal

	sort.SliceStable(langs, func(i, j int) bool {
		return langs[i].Remainder > langs[j].Remainder
	})
	for i := 0; i < remaining; i++ {
		langs[i].Assigned++
	}
	// Restore sort by original percentage descending
	sort.SliceStable(langs, func(i, j int) bool {
		return langs[i].Percent > langs[j].Percent
	})

	// Flatten squares left-to-right, top-to-bottom
	var flat []string
	for _, lang := range langs {
		for i := 0; i < lang.Assigned; i++ {
			flat = append(flat, lang.Emoji)
		}
	}

	// Format into rows × cols
	var builder strings.Builder
	for i := 0; i < len(flat); i++ {
		builder.WriteString(flat[i])
		if (i+1)%cols == 0 {
			builder.WriteString("\n")
		}
	}

	langColors := make(map[string]string)
	for _, lang := range langs {
		langColors[lang.Name] = lang.Emoji
	}

	return builder.String(), langColors
}

func padRight(str string, width int) string {
	for len(str) < width {
		str += " "
	}
	return str
}

func FormatLanguageStatsBlock(
	langData map[string][]float64,
	langColors map[string]string,
) string {
	type LangStat struct {
		Name    string
		Color   string
		Lines   float64
		Size    float64
		Percent float64
	}

	var stats []LangStat

	for name, data := range langData {
		stats = append(stats, LangStat{
			Name:    name,
			Color:   langColors[name],
			Size:    data[0],
			Lines:   data[1],
			Percent: data[2],
		})
	}

	// Sort by percentage descending
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Percent > stats[j].Percent
	})

	// Compute maximum lengths for each column
	maxName := 0
	maxLines := 0
	maxSize := 0

	lineStrs := make([]string, len(stats))
	sizeStrs := make([]string, len(stats))

	for i, s := range stats {
		if len(s.Name) > maxName {
			maxName = len(s.Name)
		}
		lines := floatToString(s.Lines) + " lines"
		size := floatToFilesize(s.Size, 1)

		lineStrs[i] = lines
		sizeStrs[i] = size

		if len(lines) > maxLines {
			maxLines = len(lines)
		}
		if len(size) > maxSize {
			maxSize = len(size)
		}
	}

	var sb strings.Builder
	for i, s := range stats {
		line := lineStrs[i]
		size := sizeStrs[i]
		percent := fmt.Sprintf("%5.2f%%", s.Percent)

		sb.WriteString(fmt.Sprintf("%s %s  %s  %s  %s\n",
			s.Color,
			padRight(s.Name, maxName),
			padRight(line, maxLines),
			padRight(size, maxSize),
			percent,
		))
	}

	return sb.String()
}

func main() {
	plan, _ := ioutil.ReadFile("github-metrics.json")
	var data Metrics
	if err := json.Unmarshal(plan, &data); err != nil {
		panic(err)
	}

	languageData := data.Plugins.Languages
	var extractedData = make(map[string][]float64)
	totalSize := float64(0)
	for lang, lineCount := range languageData.Lines {
		if lineCount > 0 {
			totalSize += float64(languageData.Stats[lang])
			extractedData[lang] = []float64{
				float64(languageData.Stats[lang]),
				float64(lineCount),
				0, // Placeholder for percentage
			}
		}
	}
	// Choose 8 languages by size
	if len(extractedData) > 8 {
		type langSize struct {
			lang string
			size float64
		}
		var sizes []langSize
		for lang, stats := range extractedData {
			sizes = append(sizes, langSize{lang: lang, size: stats[0]})
		}
		// Sort by size
		sort.Slice(sizes, func(i, j int) bool {
			return sizes[i].size > sizes[j].size
		})
		// Keep only the top 8
		if len(sizes) > 8 {
			sizes = sizes[:8]
		}
		// Create a new map with only the top 8 languages
		topLanguages := make(map[string][]float64)
		for _, size := range sizes {
			topLanguages[size.lang] = extractedData[size.lang]
		}
		extractedData = topLanguages

		// Recalculate total size for the top 8 languages
		totalSize = 0
		for _, stats := range extractedData {
			totalSize += stats[0]
		}
	}
	for _, stats := range extractedData {
		if totalSize > 0 {
			stats[2] = (stats[0] / totalSize) * 100 // Calculate percentage
		}
		//log.Printf("%s: %s (%s, %.2f%%)", lang, floatToFilesize(stats[0], 1), floatToString(stats[1]), stats[2])
	}

	grid, langColors := GenerateEmojiGrid(extractedData, 10, 10)
	statBlock := FormatLanguageStatsBlock(extractedData, langColors)
	log.Println("\n" + grid)
	log.Println("\nLanguage Stats:\n" + statBlock)

	// Open the template file
	template, err := ioutil.ReadFile("README.md.template")
	if err != nil {
		log.Fatalf("Error reading template file: %v", err)
	}
	// Prepare for proper Markdown formatting
	grid = strings.ReplaceAll(grid, "\n", "<br>")
	// Remove trailing newline from the statBlock

	// Replace placeholders in the template
	readme := strings.ReplaceAll(string(template), "{{LANGUAGE_GRID}}", grid)
	readme = strings.ReplaceAll(readme, "{{LANGUAGE_STATS}}", statBlock)
	readme = strings.ReplaceAll(readme, "{{VERSION}}", data.Meta.Version)
	readme = strings.ReplaceAll(readme, "{{UPDATE_DATE}}", data.Meta.Generated)
	readme = strings.ReplaceAll(readme, "{{CODEBASE_SIZE}}", floatToFilesize(totalSize, 2))
	readme = strings.ReplaceAll(readme, "{{FILE_COUNT}}", strconv.Itoa(languageData.Files))
	readme = strings.ReplaceAll(readme, "{{COMMIT_COUNT}}", strconv.Itoa(languageData.Commits))
	// Write the output to README.md
	err = ioutil.WriteFile("README.md", []byte(readme), 0644)
	if err != nil {
		log.Fatalf("Error writing README.md: %v", err)
	}
	log.Println("README.md generated successfully!")
}
