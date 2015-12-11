package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/daviddengcn/go-colortext"
	"github.com/phrase/phraseapp-go/phraseapp"
)

type PushCommand struct {
	Credentials
}

func (cmd *PushCommand) Run() error {
	if cmd.Debug {
		// suppresses content output
		cmd.Debug = false
		Debug = true
	}
	client, err := ClientFromCmdCredentials(cmd.Credentials)
	if err != nil {
		return err
	}

	sources, err := SourcesFromConfig(cmd)
	if err != nil {
		return err
	}

	for _, source := range sources {
		err := source.Push(client)
		if err != nil {
			return err
		}
	}
	return nil
}

type Sources []*Source

type Source struct {
	File          string      `yaml:"file,omitempty"`
	ProjectID     string      `yaml:"project_id,omitempty"`
	AccessToken   string      `yaml:"access_token,omitempty"`
	FileFormat    string      `yaml:"file_format,omitempty"`
	Params        *phraseapp.UploadParams `yaml:"params"`

	RemoteLocales []*phraseapp.Locale
	Extension     string
}


var separator = string(os.PathSeparator)

func (source *Source) CheckPreconditions() error {
	if err := ValidPath(source.File, source.FileFormat, ""); err != nil {
		return err
	}

	duplicatedPlaceholders := []string{}
	for _, name := range []string{"<locale_name>", "<locale_code>", "<tag>"} {
		if strings.Count(source.File, name) > 1 {
			duplicatedPlaceholders = append(duplicatedPlaceholders, name)
		}
	}

	starCount := strings.Count(source.File, "*")

	if starCount > 1 {
		duplicatedPlaceholders = append(duplicatedPlaceholders, "*")
	}

	if len(duplicatedPlaceholders) > 0 {
		dups := strings.Join(duplicatedPlaceholders, ", ")
		return fmt.Errorf(fmt.Sprintf("%s can only occur once in a file pattern!", dups))
	}

	return nil
}

func (source *Source) Push(client *phraseapp.Client) error {
	if err := source.CheckPreconditions(); err != nil {
		return err
	}

	source.Extension = filepath.Ext(source.File)

	remoteLocales, err := RemoteLocales(client, source.ProjectID)
	if err != nil {
		return err
	}
	source.RemoteLocales = remoteLocales

	localeFiles, err := source.LocaleFiles()
	if err != nil {
		return err
	}

	for _, localeFile := range localeFiles {
		fmt.Println("Uploading", localeFile.RelPath())

		if !localeFile.ExistsRemote {
			localeDetails, err := source.createLocale(client, localeFile)
			if err == nil {
				localeFile.ID = localeDetails.ID
				localeFile.RFC = localeDetails.Code
				localeFile.Name = localeDetails.Name
			} else {
				fmt.Printf("failed to create locale: %s\n", err)
				continue
			}
		}

		err = source.uploadFile(client, localeFile)
		if err != nil {
			return err
		} else {
			sharedMessage("push", localeFile)
		}

		if Debug {
			fmt.Fprintln(os.Stderr, strings.Repeat("-", 10))
		}

	}

	return nil
}

func (source *Source) createLocale(client *phraseapp.Client, localeFile *LocaleFile) (*phraseapp.LocaleDetails, error) {
	if localeFile.RFC == "" {
		return nil, fmt.Errorf("no locale code specified")
	}

	localeParams := new(phraseapp.LocaleParams)

	if localeFile.Name != "" {
		localeParams.Name = &localeFile.Name
	} else if localeFile.RFC != "" {
		localeParams.Name = &localeFile.RFC
	}

	localeName := source.replacePlaceholderInParams(localeFile)
	if localeName != "" && localeName != localeFile.RFC {
		localeParams.Name = &localeName
	}

	if localeFile.RFC != "" {
		localeParams.Code = &localeFile.RFC
	}

	localeDetails, err := client.LocaleCreate(source.ProjectID, localeParams)
	if err != nil {
		return nil, err
	}
	return localeDetails, nil
}

func (source *Source) replacePlaceholderInParams(localeFile *LocaleFile) string {
	if localeFile.RFC != "" && strings.Contains(source.GetLocaleID(), "<locale_code>") {
		return strings.Replace(source.GetLocaleID(), "<locale_code>", localeFile.RFC, 1)
	}
	return ""
}

func (source *Source) uploadFile(client *phraseapp.Client, localeFile *LocaleFile) error {
	if Debug {
		fmt.Fprintln(os.Stdout, "Source file pattern:", source.File)
		fmt.Fprintln(os.Stdout, "Actual file location:", localeFile.Path)
	}

	params := new(phraseapp.UploadParams)
	*params = *source.Params

	params.File = &localeFile.Path

	if params.LocaleID == nil {
		switch {
		case localeFile.ID != "":
			params.LocaleID = &localeFile.ID
		case localeFile.RFC != "":
			params.LocaleID = &localeFile.RFC
		}
	}

	aUpload, err := client.UploadCreate(source.ProjectID, params)
	if err != nil {
		return err
	}

	printSummary(&aUpload.Summary)

	return nil
}

func (source *Source) LocaleFiles() (LocaleFiles, error) {
	filePaths, err := source.glob()
	if err != nil {
		return nil, err
	}

	reducer := Reducer{
		Tokens: Tokenize(source.File),
	}

	var localeFiles LocaleFiles
	for _, path := range filePaths {

		pathTokens := Tokenize(path)
		if len(pathTokens) < len(reducer.Tokens) {
			continue
		}

		temporaryLocaleFile, err := reducer.Reduce(pathTokens)
		if err != nil {
			return nil, err
		}

		localeFile, err := source.generateLocaleForFile(temporaryLocaleFile, path)
		if err != nil {
			return nil, err
		}

		if Debug {
			fmt.Println(fmt.Sprintf(
				"RFC:'%s', Name:'%s', Tag;'%s', Pattern:'%s'",
				localeFile.RFC, localeFile.Name, localeFile.Tag,
			))
		}

		localeFiles = append(localeFiles, localeFile)
	}

	if len(localeFiles) <= 0 {
		abs, err := filepath.Abs(source.File)
		if err != nil {
			return nil, err
		}
		errmsg := fmt.Sprintf("Could not find any files on your system that matches: '%s'", abs)
		ReportError("Push Error", errmsg)
		return nil, fmt.Errorf(errmsg)
	}
	return localeFiles, nil
}

func (source *Source) generateLocaleForFile(localeFile *LocaleFile, path string) (*LocaleFile, error) {
	locale := source.getRemoteLocaleForLocaleFile(localeFile)
	if locale != nil {
		localeFile.ExistsRemote = true
		localeFile.RFC = locale.Code
		localeFile.Name = locale.Name
		localeFile.ID = locale.ID
	}

	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	localeFile.Path = absolutePath

	return localeFile, nil
}

func (source *Source) getRemoteLocaleForLocaleFile(localeFile *LocaleFile) *phraseapp.Locale {
	for _, remote := range source.RemoteLocales {
		if remote.Name == source.GetLocaleID() || remote.ID == source.GetLocaleID() {
			return remote
		}

		localeName := source.replacePlaceholderInParams(localeFile)
		if localeName != "" && strings.Contains(remote.Name, localeName) {
			return remote
		}

		if remote.Name == localeFile.Name {
			return remote
		}

		if remote.Name == localeFile.RFC {
			return remote
		}
	}
	return nil
}

func (source *Source) glob() ([]string, error) {
	withoutPlaceholder := placeholderRegexp.ReplaceAllString(source.File, "*")
	tokens := Tokenize(withoutPlaceholder)

	fileHead := tokens[len(tokens)-1]
	if strings.HasPrefix(fileHead, ".") {
		tokens[len(tokens)-1] = "*" + fileHead
	}
	pattern := strings.Join(tokens, separator)

	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	if Debug {
		fmt.Fprintln(os.Stderr, "Found", len(files), "files matching the source pattern", pattern)
	}

	return files, nil
}

type Reducer struct {
	Tokens []string
}

func Tokenize(s string) []string {
	tokens := []string{}
	for _, token := range strings.Split(s, separator) {
		if token == "." || token == "" {
			continue
		}
		tokens = append(tokens, token)
	}
	return tokens
}

func (reducer *Reducer) Reduce(pathTokens []string) (*LocaleFile, error) {
	tagged := reducer.Tag(pathTokens)

	name := tagged["locale_name"]
	rfc := tagged["locale_code"]
	tag := tagged["tag"]

	localeFile := &LocaleFile{}
	if name != "" {
		localeFile.Name = name
	}

	if rfc != "" {
		localeFile.RFC = rfc
	}

	if tag != "" {
		localeFile.Tag = tag
	}

	return localeFile, nil
}

func (reducer *Reducer) convertToGroupRegexp(part string) string {
	groups := placeholderRegexp.FindAllString(part, -1)
	if len(groups) <= 0 {
		return ""
	}

	part = strings.Replace(part, ".", "[.]", -1)

	for _, group := range groups {
		replacer := fmt.Sprintf("(?P%s.+)", group)
		part = strings.Replace(part, group, replacer, 1)
	}

	return part
}

func (reducer *Reducer) Tag(pathTokens []string) map[string]string {
	tagged := map[string]string{}

	for idx, token := range reducer.Tokens {
		match := reducer.convertToGroupRegexp(token)
		if match == "" {
			continue
		}

		tmpRegexp, err := regexp.Compile(match)
		if err != nil {
			continue
		}

		namedMatches := tmpRegexp.SubexpNames()
		subMatches := tmpRegexp.FindStringSubmatch(pathTokens[idx])
		for i, subMatch := range subMatches {
			if subMatch != "" {
				tagged[namedMatches[i]] = strings.Trim(subMatch, separator)
			}
		}
	}

	return tagged
}

// Print out
func printSummary(summary *phraseapp.SummaryType) {
	newItems := []int64{
		summary.LocalesCreated,
		summary.TranslationsUpdated,
		summary.TranslationKeysCreated,
		summary.TranslationsCreated,
	}
	var changed bool
	for _, item := range newItems {
		if item > 0 {
			changed = true
		}
	}
	if changed || Debug {
		printMessage("Locales created: ", fmt.Sprintf("%d", summary.LocalesCreated))
		printMessage(" - Keys created: ", fmt.Sprintf("%d", summary.TranslationKeysCreated))
		printMessage(" - Translations created: ", fmt.Sprintf("%d", summary.TranslationsCreated))
		printMessage(" - Translations updated: ", fmt.Sprintf("%d", summary.TranslationsUpdated))
		fmt.Print("\n")
	}
}

func printMessage(msg, stat string) {
	fmt.Print(msg)
	ct.Foreground(ct.Green, true)
	fmt.Print(stat)
	ct.ResetColor()
}

// Configuration
type PushConfig struct {
	Phraseapp struct {
		AccessToken string `yaml:"access_token"`
		ProjectID   string `yaml:"project_id"`
		FileFormat  string `yaml:"file_format,omitempty"`
		Push        struct {
			Sources Sources
		}
	}
}

func SourcesFromConfig(cmd *PushCommand) (Sources, error) {
	content, err := ConfigContent()
	if err != nil {
		return nil, err
	}

	var config *PushConfig

	err = yaml.Unmarshal([]byte(content), &config)
	if err != nil {
		return nil, err
	}

	token := config.Phraseapp.AccessToken
	if cmd.Token != "" {
		token = cmd.Token
	}
	projectId := config.Phraseapp.ProjectID
	fileFormat := config.Phraseapp.FileFormat

	if &config.Phraseapp.Push == nil || config.Phraseapp.Push.Sources == nil {
		errmsg := "no sources for upload specified"
		ReportError("Push Error", errmsg)
		return nil, fmt.Errorf(errmsg)
	}

	sources := config.Phraseapp.Push.Sources

	validSources := []*Source{}
	for _, source := range sources {
		if source == nil {
			continue
		}
		if source.ProjectID == "" {
			source.ProjectID = projectId
		}
		if source.AccessToken == "" {
			source.AccessToken = token
		}
		if source.Params == nil {
			source.Params = new(phraseapp.UploadParams)
		}

		if source.Params.FileFormat == nil {
			switch {
			case source.FileFormat != "":
				source.Params.FileFormat = &source.FileFormat
			case fileFormat != "":
				source.Params.FileFormat = &fileFormat
			}
		}
		validSources = append(validSources, source)
	}

	if len(validSources) <= 0 {
		errmsg := "no sources could be identified! Refine the sources list in your config"
		ReportError("Push Error", errmsg)
		return nil, fmt.Errorf(errmsg)
	}

	return validSources, nil
}

func (source *Source) GetLocaleID() string {
	if source.Params != nil && source.Params.LocaleID != nil {
		return *source.Params.LocaleID
	}
	return ""
}
