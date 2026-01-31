package analyzer

import (
	"fmt"
	"time"
)

type SizeCalculator struct {
	bytesPerSample int
	retentionDays  int
	scrapeInterval time.Duration
	samplesPerDay  int
}

type SizeConfig struct {
	BytesPerSample int
	RetentionDays  int
	ScrapeInterval time.Duration
}

func NewSizeCalculator(cfg SizeConfig) *SizeCalculator {
	if cfg.BytesPerSample == 0 {
		cfg.BytesPerSample = 2
	}
	if cfg.RetentionDays == 0 {
		cfg.RetentionDays = 30
	}
	if cfg.ScrapeInterval == 0 {
		cfg.ScrapeInterval = 15 * time.Second
	}

	samplesPerDay := int(24 * time.Hour / cfg.ScrapeInterval)

	return &SizeCalculator{
		bytesPerSample: cfg.BytesPerSample,
		retentionDays:  cfg.RetentionDays,
		scrapeInterval: cfg.ScrapeInterval,
		samplesPerDay:  samplesPerDay,
	}
}

// EstimateSize calculates storage size in bytes for a metric with given cardinality
// Formula: cardinality × samples_per_day × retention_days × bytes_per_sample
func (c *SizeCalculator) EstimateSize(cardinality int) int64 {
	return int64(cardinality) * int64(c.samplesPerDay) * int64(c.retentionDays) * int64(c.bytesPerSample)
}

func (c *SizeCalculator) SamplesPerDay() int {
	return c.samplesPerDay
}

func (c *SizeCalculator) RetentionDays() int {
	return c.retentionDays
}

func (c *SizeCalculator) BytesPerSample() int {
	return c.bytesPerSample
}

func FormatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
