package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"admission-api/internal/gaokaoimport"
	"admission-api/internal/platform/config"
	"admission-api/internal/platform/db"
)

func main() {
	dataDir := flag.String("data-dir", "", "directory containing CSV dump files")
	truncate := flag.Bool("truncate", false, "truncate gaokao tables before import")
	only := flag.String("only", "", "comma-separated import phases: base,schools,majors,school_profile,major_profile,school_tags,major_tags,policies,school_major_catalog,subject_requirements,school_major_groups,facts")
	skipXGK := flag.Bool("skip-xgk", false, "skip xgk/elective related imports")
	sampleRows := flag.Int("sample-rows", 0, "import only the first N rows from each CSV file for repeatable sample runs")
	maxReadRows := flag.Int("max-read-rows", 0, "read at most N rows from each CSV file before filtering; useful for fast dev imports")
	profile := flag.String("profile", "", "import profile: dev")
	flag.Parse()

	if strings.TrimSpace(*dataDir) == "" {
		log.Fatal("data-dir is required")
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	ctx := context.Background()

	database, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	opts := gaokaoimport.Options{
		DataDir:     *dataDir,
		Truncate:    *truncate,
		SkipXGK:     *skipXGK,
		SampleRows:  *sampleRows,
		MaxReadRows: *maxReadRows,
		Profile:     *profile,
	}
	if strings.TrimSpace(*only) != "" {
		opts.Only = strings.Split(*only, ",")
	}

	importer := gaokaoimport.New(database.Pool(), &opts)
	if err := importer.Run(ctx); err != nil {
		database.Close()
		log.Printf("import failed: %v", err)
		os.Exit(1)
	}
	database.Close()

	fmt.Println("gaokao import completed")
}
