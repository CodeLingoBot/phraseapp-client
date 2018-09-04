package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/phrase/phraseapp-client/internal/api"
	"github.com/phrase/phraseapp-client/internal/paths"
	"github.com/phrase/phraseapp-client/internal/placeholders"
	"github.com/phrase/phraseapp-client/internal/print"
	"github.com/phrase/phraseapp-go/phraseapp"
)

const (
	timeoutInMinutes = 30
)

type PullCommand struct {
	phraseapp.Config
	Branch string `cli:"opt --branch"`
}

func (cmd *PullCommand) Run() error {
	if cmd.Config.Debug {
		// suppresses content output
		cmd.Config.Debug = false
		Debug = true
	}
	client, err := newClient(cmd.Config.Credentials, cmd.Config.Debug)
	if err != nil {
		return err
	}

	targets, err := TargetsFromConfig(cmd.Config)
	if err != nil {
		return err
	}

	projectIdToLocales, err := LocalesForProjects(client, targets, cmd.Branch)
	if err != nil {
		return err
	}

	for _, target := range targets {
		val, ok := projectIdToLocales[LocaleCacheKey{target.ProjectID, cmd.Branch}]
		if !ok || len(val) == 0 {
			return fmt.Errorf("Could not find any locales for project %q", target.ProjectID)
		}
		target.RemoteLocales = val
	}

	for _, target := range targets {
		err := target.Pull(client, cmd.Branch)
		if err != nil {
			return err
		}
	}

	return nil
}

type PullParams struct {
	phraseapp.LocaleDownloadParams
	LocaleID string
}

func (target *Target) Pull(client *phraseapp.Client, branch string) error {
	if err := target.CheckPreconditions(); err != nil {
		return err
	}

	localeFiles, err := target.LocaleFiles()
	if err != nil {
		return err
	}

	startedAt := time.Now()
	for _, localeFile := range localeFiles {
		if time.Now().Sub(startedAt).Minutes() >= timeoutInMinutes {
			return fmt.Errorf("Timeout of %d minutes exceeded", timeoutInMinutes)
		}

		err := createFile(localeFile.Path)
		if err != nil {
			return err
		}

		err = target.DownloadAndWriteToFile(client, localeFile, branch)
		if err != nil {
			return fmt.Errorf("%s for %s", err, localeFile.Path)
		} else {
			print.Success("Downloaded %s to %s", localeFile.Message(), localeFile.RelPath())
		}
		if Debug {
			fmt.Fprintln(os.Stderr, strings.Repeat("-", 10))
		}
	}

	return nil
}

func (target *Target) DownloadAndWriteToFile(client *phraseapp.Client, localeFile *LocaleFile, branch string) error {
	downloadParams := &phraseapp.LocaleDownloadParams{Branch: &branch}
	if target.Params != nil {
		*downloadParams = target.Params.LocaleDownloadParams
		downloadParams.Branch = &branch
	}

	if downloadParams.FileFormat == nil {
		downloadParams.FileFormat = &localeFile.FileFormat
	}

	if Debug {
		fmt.Fprintln(os.Stderr, "Target file pattern:", target.File)
		fmt.Fprintln(os.Stderr, "Actual file path", localeFile.Path)
		fmt.Fprintln(os.Stderr, "LocaleID", localeFile.ID)
		fmt.Fprintln(os.Stderr, "ProjectID", target.ProjectID)
		fmt.Fprintln(os.Stderr, "FileFormat", downloadParams.FileFormat)
		fmt.Fprintln(os.Stderr, "ConvertEmoji", downloadParams.ConvertEmoji)
		fmt.Fprintln(os.Stderr, "IncludeEmptyTranslations", downloadParams.IncludeEmptyTranslations)
		fmt.Fprintln(os.Stderr, "KeepNotranslateTags", downloadParams.KeepNotranslateTags)
		fmt.Fprintln(os.Stderr, "Tag", downloadParams.Tag)
		fmt.Fprintln(os.Stderr, "FormatOptions", downloadParams.FormatOptions)
	}

	resp, err := api.LocaleDownload(client, target.ProjectID, localeFile.ID, downloadParams)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	remaining, err := strconv.Atoi(resp.Header.Get("X-Rate-Limit-Remaining"))
	if err != nil {
		remaining = 0
	}

	if remaining == 0 {
		reset, err := strconv.ParseInt(resp.Header.Get("X-Rate-Limit-Reset"), 10, 64)
		if err != nil {
			reset = time.Now().Add(time.Second * 10).Unix()
		}

		resetTime := time.Unix(reset, 0).Sub(time.Now())
		fmt.Printf("Rate limit exceeded. Download will continue in %d seconds\n", int64(resetTime.Seconds()))
		time.Sleep(resetTime)
	}

	err = ioutil.WriteFile(localeFile.Path, content, 0700)
	return err
}

func (target *Target) LocaleFiles() (LocaleFiles, error) {
	files := []*LocaleFile{}

	if target.GetLocaleID() != "" {
		// a specific locale was requested
		remoteLocale, err := target.localeForRemote()
		if err != nil {
			return nil, err
		}

		localeFile, err := createLocaleFile(target, remoteLocale)
		if err != nil {
			return nil, err
		}

		files = append(files, localeFile)

	} else if placeholders.ContainsLocalePlaceholder(target.File) {
		// multiple locales were requested
		for _, remoteLocale := range target.RemoteLocales {
			localeFile, err := createLocaleFile(target, remoteLocale)
			if err != nil {
				return nil, err
			}

			files = append(files, localeFile)
		}
	} else {
		// no local files match remote locale
		return nil, fmt.Errorf("Could not find any files on your system that matches the locales for porject %q.", target.ProjectID)
	}

	return files, nil
}

func createLocaleFile(target *Target, remoteLocale *phraseapp.Locale) (*LocaleFile, error) {
	localeFile := &LocaleFile{
		Name:       remoteLocale.Name,
		ID:         remoteLocale.ID,
		Code:       remoteLocale.Code,
		Tag:        target.GetTag(),
		FileFormat: target.GetFormat(),
		Path:       target.File,
	}

	absPath, err := target.ReplacePlaceholders(localeFile)
	if err != nil {
		return nil, err
	}

	localeFile.Path = absPath
	return localeFile, nil
}

func createFile(path string) error {
	err := paths.Exists(path)
	if err != nil {
		absDir := filepath.Dir(path)
		err := paths.Exists(absDir)
		if err != nil {
			os.MkdirAll(absDir, 0700)
		}

		f, err := os.Create(path)
		if err != nil {
			return err
		}
		defer f.Close()
	}

	return nil
}
