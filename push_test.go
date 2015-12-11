package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"github.com/phrase/phraseapp-go/phraseapp"
)

func getBaseSource() *Source {
	source := &Source{
		File:        "./tests/<locale_code>.yml",
		ProjectID:   "project-id",
		AccessToken: "access-token",
		FileFormat:  "yml",
		Extension:   "",
		Params: new(phraseapp.UploadParams),
		RemoteLocales: getBaseLocales(),
	}
	source.Extension = filepath.Ext(source.File)
	return source
}

func TestPushPreconditions(t *testing.T) {
	fmt.Println("Push#Source#CheckPreconditions")
	source := getBaseSource()
	for _, file := range []string{
		"",
		"no_extension",
		"./<locale_code>/<locale_code>.yml",
		"./*/*/en.yml",
	} {
		source.File = file
		if err := source.CheckPreconditions(); err == nil {
			t.Errorf("CheckPrecondition did not fail for pattern: '%s'", file)
		}
	}

	for _, file := range []string{
		"./<tag>/<locale_code>.yml",
		"./*/en.yml",
		"./*/<locale_name>/<locale_code>/<tag>.yml",
	} {
		source.File = file
		if err := source.CheckPreconditions(); err != nil {
			t.Errorf("CheckPrecondition should not fail with: %s", err.Error())
		}
	}
}

func TestSourceFields(t *testing.T) {
	fmt.Println("Push#Source#Fields")
	source := getBaseSource()

	if source.File != "./tests/<locale_code>.yml" {
		t.Errorf("Expected File to be %s and not %s", "./tests/<locale_code>.yml", source.File)
	}

	if source.AccessToken != "access-token" {
		t.Errorf("Expected AccesToken to be %s and not %s", "access-token", source.AccessToken)
	}

	if source.ProjectID != "project-id" {
		t.Errorf("Expected ProjectID to be %s and not %s", "project-id", source.ProjectID)
	}

	if source.FileFormat != "yml" {
		t.Errorf("Expected FileFormat to be %s and not %s", "yml", source.FileFormat)
	}

}

func TestSourceLocaleFilesOne(t *testing.T) {
	fmt.Println("Push#Source#LocaleFiles#1")
	source := getBaseSource()
	localeFiles, err := source.LocaleFiles()

	if err != nil {
		t.Errorf("Should not fail with: %s", err.Error())
	}

	absPath, _ := filepath.Abs("./tests/en.yml")
	expectedFiles := []*LocaleFile{
		&LocaleFile{
			Name: "",
			RFC:  "en",
			ID:   "",
			Path: absPath,
		},
	}

	if len(localeFiles) == len(expectedFiles) {
		if err = compareLocaleFiles(localeFiles, expectedFiles); err != nil {
			t.Errorf(err.Error())
		}
	} else {
		t.Errorf(".LocaleFiles should contain %s and not %s", expectedFiles, localeFiles)
	}
}

func TestSourceLocaleFilesTwo(t *testing.T) {
	fmt.Println("Push#Source#LocaleFiles#2")
	source := getBaseSource()
	source.File = "./**/<locale_name>.yml"

	localeFiles, err := source.LocaleFiles()

	if err != nil {
		t.Errorf("Should not fail with: %s", err.Error())
	}

	absPath, _ := filepath.Abs("./tests/en.yml")
	expectedFiles := []*LocaleFile{
		&LocaleFile{
			Name: "en",
			RFC:  "",
			ID:   "",
			Path: absPath,
		},
	}

	if len(localeFiles) == len(expectedFiles) {
		if err = compareLocaleFiles(localeFiles, expectedFiles); err != nil {
			t.Errorf(err.Error())
		}
	} else {
		t.Errorf("LocaleFiles should contain %s and not %s", expectedFiles, localeFiles)
	}
}

func TestReplacePlaceholderInParams(t *testing.T) {
	fmt.Println("Push#Source#ReplacePlaceholderInParams")
	source := getBaseSource()
	lid := "<locale_code>"
	source.Params.LocaleID = &lid
	localeFile := &LocaleFile{
		Name: "en",
		RFC:  "en",
		ID:   "",
		Path: "",
	}
	s := source.replacePlaceholderInParams(localeFile)
	if s != "en" {
		t.Errorf("Expected LocaleId to equal '%s' but was '%s'", "en", s)
		t.Fail()
	}
}
func TestGetRemoteLocaleForLocaleFile(t *testing.T) {
	fmt.Println("Push#Source#getRemoteLocaleForLocaleFile")
	source := getBaseSource()
	localeFile := &LocaleFile{
		Name: "english",
		RFC:  "en",
		ID:   "",
		Path: "",
	}
	locale := source.getRemoteLocaleForLocaleFile(localeFile)
	if locale.Name != localeFile.Name {
		t.Errorf("Expected LocaleName to equal '%s' but was '%s'", "ennglish", localeFile.Name)
		t.Fail()
	}
	if locale.Code != localeFile.RFC {
		t.Errorf("Expected LocaleId to equal '%s' but was '%s'", "en", localeFile.RFC)
		t.Fail()
	}
}

func TestGenerateLocaleForFile(t *testing.T) {
	fmt.Println("Push#Source#generateLocaleForFile")
	source := getBaseSource()

	validPathOnFileSystem := "abc/defg"
	localeFile := &LocaleFile{
		Name: "english",
		RFC:  "en",
		ID:   "",
		Path: "",
	}
	newLocaleFile, err := source.generateLocaleForFile(localeFile, validPathOnFileSystem)
	if err != nil {
		t.Errorf(err.Error())
		t.Fail()
	}

	if newLocaleFile.ExistsRemote == false {
		t.Errorf("Expected that Locale exists remote but was '%b'", newLocaleFile.ExistsRemote)
		t.Fail()
	}

	if newLocaleFile.RFC != "en" {
		t.Errorf("Expected LocaleCode to equal '%s' but was '%s'", "en", newLocaleFile.RFC)
		t.Fail()
	}

	if newLocaleFile.Name != "english" {
		t.Errorf("Expected LocaleName to equal '%s' but was '%s'", "english", newLocaleFile.Name)
		t.Fail()
	}

	if newLocaleFile.ID != "en-locale-id" {
		t.Errorf("Expected LocaleId to equal '%s' but was '%s'", "en-locale-id", newLocaleFile.ID)
		t.Fail()
	}
}

type Pattern struct {
	File         string
	Ext          string
	TestPath     string
	ExpectedRFC  string
	ExpectedName string
	ExpectedTag  string
}

func TestReducerPatterns(t *testing.T) {
	fmt.Println("Push#Reducer")
	for idx, pattern := range []*Pattern{
		&Pattern{
			File:        "./.abc/<locale_code>.yml",
			Ext:         "yml",
			TestPath:    ".abc/en.yml",
			ExpectedRFC: "en",
		},
		&Pattern{
			File:        "./abc+/defg./}{x/][etc??/<locale_code>.yml",
			Ext:         "yml",
			TestPath:    "abc+/defg./}{x/][etc??/en.yml",
			ExpectedRFC: "en",
		},
		&Pattern{
			File:     "./*.yml",
			Ext:      "yml",
			TestPath: "en.yml",
		},
		&Pattern{
			File:     "./locales/?*.yml",
			Ext:      "yml",
			TestPath: "locales/?en.yml",
		},
		&Pattern{
			File:     "./locales/en.yml",
			Ext:      "yml",
			TestPath: "locales/en.yml",
		},
		&Pattern{
			File:     "./locales/.yml",
			Ext:      "yml",
			TestPath: "locales/en.yml",
		},
		&Pattern{
			File:        "./abc/defg/<locale_code>.lproj/.strings",
			Ext:         "strings",
			TestPath:    "abc/defg/en.lproj/Localizable.strings",
			ExpectedRFC: "en",
		},
		&Pattern{
			File:        "./locales/<locale_code>.yml",
			Ext:         "yml",
			TestPath:    "locales/en.yml",
			ExpectedRFC: "en",
		},
		&Pattern{
			File:        "./config/<tag>/<locale_code>.yml",
			Ext:         "yml",
			TestPath:    "config/abc/en.yml",
			ExpectedRFC: "en",
			ExpectedTag: "abc",
		},
		&Pattern{
			File:         "./config/<locale_name>/<locale_code>.yml",
			Ext:          "yml",
			TestPath:     "config/german/de.yml",
			ExpectedRFC:  "de",
			ExpectedTag:  "",
			ExpectedName: "german",
		},
		&Pattern{
			File:        "./config/<locale_code>/*.yml",
			Ext:         "yml",
			TestPath:    "config/en/english.yml",
			ExpectedRFC: "en",
		},
		&Pattern{
			File:        "./no_tag/<tag>/<locale_code>.lproj/Localizable.strings",
			Ext:         "strings",
			TestPath:    "no_tag/abc/en.lproj/Localizable.strings",
			ExpectedRFC: "en",
			ExpectedTag: "abc",
		},
		&Pattern{
			File:        "./abc/<locale_code>.lproj/.strings",
			Ext:         "strings",
			TestPath:    "abc/en.lproj/Localizable.strings",
			ExpectedRFC: "en",
		},
		&Pattern{
			File:        "./abc/<locale_code>.lproj/<tag>.strings",
			Ext:         "strings",
			TestPath:    "abc/en.lproj/MyStoryboard.strings",
			ExpectedRFC: "en",
			ExpectedTag: "MyStoryboard",
		},
		&Pattern{
			File:        "./*/<tag>/<locale_code>-values/Strings.xml",
			Ext:         "xml",
			TestPath:    "no_tag/abc/en-values/Strings.xml",
			ExpectedRFC: "en",
			ExpectedTag: "abc",
		},
		&Pattern{
			File:        "./*/<tag>/play.<locale_code>",
			Ext:         "<locale_code>",
			TestPath:    "no_tag/abc/play.en",
			ExpectedRFC: "en",
			ExpectedTag: "abc",
		},
		&Pattern{
			File:         "./*/<tag>/<locale_name>.<locale_code>",
			Ext:          "<locale_code>",
			TestPath:     "no_tag/abc/play.en",
			ExpectedRFC:  "en",
			ExpectedTag:  "abc",
			ExpectedName: "play",
		},
		&Pattern{
			File:         "./abc/<locale_code>/<tag>.<locale_name>",
			Ext:          "strings",
			TestPath:     "abc/en/MyStoryboard.english",
			ExpectedRFC:  "en",
			ExpectedTag:  "MyStoryboard",
			ExpectedName: "english",
		},
	} {
		reducer := &Reducer{
			Tokens: Tokenize(pattern.File),
		}
		fmt.Println(strings.Repeat("-", 10))
		fmt.Println(idx+1, "\n  SourceFile:", pattern.File, "\n  TestPath:", pattern.TestPath)

		pathTokens := Tokenize(pattern.TestPath)
		localeFile, err := reducer.Reduce(pathTokens)
		if err != nil {
			fmt.Println(fmt.Sprintf("\tRFC:'%s', Name:'%s', Tag:'%s'", localeFile.RFC, localeFile.Name, localeFile.Tag))
			t.Errorf(err.Error())
			t.Fail()
		}

		if localeFile.RFC != pattern.ExpectedRFC {
			t.Errorf("Expected RFC to equal '%s' but was '%s' Pattern: %d", pattern.ExpectedRFC, localeFile.RFC, idx+1)
			t.Fail()
		}

		if localeFile.Tag != pattern.ExpectedTag {
			t.Errorf("Expected Tag to equal '%s' but was '%s' Pattern: %d", pattern.ExpectedTag, localeFile.Tag, idx+1)
			t.Fail()
		}

		if localeFile.Name != pattern.ExpectedName {
			t.Errorf("Expected LocaleName to equal '%s' but was '%s' Pattern: %d", pattern.ExpectedName, localeFile.Name, idx+1)
			t.Fail()
		}
	}
	fmt.Println(strings.Repeat("-", 10))
}
