package config

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/ayushjn20/dunner/internal/logger"
	"github.com/ayushjn20/dunner/internal/util"
	"github.com/ayushjn20/dunner/pkg/docker"
	"github.com/docker/docker/api/types/mount"
	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/joho/godotenv"
	"github.com/spf13/viper"
	"gopkg.in/go-playground/validator.v9"
	en_translations "gopkg.in/go-playground/validator.v9/translations/en"
	yaml "gopkg.in/yaml.v2"
)

var log = logger.Log
var dotEnv map[string]string

var (
	uni                     *ut.UniversalTranslator
	govalidator             *validator.Validate
	trans                   ut.Translator
	defaultPermissionMode   = "r"
	validDirPermissionModes = []string{defaultPermissionMode, "wr", "rw", "w"}
)

type contextKey string

var configsKey = contextKey("dunnerConfigs")

type customValidation struct {
	tag          string
	translation  string
	validationFn func(context.Context, validator.FieldLevel) bool
}

var customValidations = []customValidation{
	{
		tag:          "mountdir",
		translation:  "mount directory '{0}' is invalid. Check format is '<valid_src_dir>:<valid_dest_dir>:<optional_mode>' and has right permission level",
		validationFn: ValidateMountDir,
	},
	{
		tag:          "follow_exist",
		translation:  "follow task '{0}' does not exist",
		validationFn: ValidateFollowTaskPresent,
	},
	{
		tag:          "parsedir",
		translation:  "mount directory '{0}' is invalid. Check if source directory path exists.",
		validationFn: ParseMountDir,
	},
}

// Task describes a single task to be run in a docker container
type Task struct {
	Name     string     `yaml:"name"`
	Image    string     `yaml:"image" validate:"required"`
	SubDir   string     `yaml:"dir"`
	Command  []string   `yaml:"command" validate:"omitempty,dive,required"`
	Commands [][]string `yaml:"commands" validate:"omitempty,dive,omitempty,dive,required"`
	Envs     []string   `yaml:"envs"`
	Mounts   []string   `yaml:"mounts" validate:"omitempty,dive,min=1,mountdir,parsedir"`
	Follow   string     `yaml:"follow" validate:"omitempty,follow_exist"`
	Args     []string   `yaml:"args"`
}

// Configs describes the parsed information from the dunner file
type Configs struct {
	Tasks map[string][]Task `validate:"required,min=1,dive,keys,required,endkeys,required,min=1,required"`
}

// Validate validates config and returns errors.
func (configs *Configs) Validate() []error {
	err := initValidator(customValidations)
	if err != nil {
		return []error{err}
	}
	valErrs := govalidator.Struct(configs)
	errs := formatErrors(valErrs, "")
	ctx := context.WithValue(context.Background(), configsKey, configs)

	// Each task is validated separately so that task name can be added in error messages
	for taskName, tasks := range configs.Tasks {
		taskValErrs := govalidator.VarCtx(ctx, tasks, "dive")
		errs = append(errs, formatErrors(taskValErrs, taskName)...)
	}
	return errs
}

func formatErrors(valErrs error, taskName string) []error {
	var errs []error
	if valErrs != nil {
		if _, ok := valErrs.(*validator.InvalidValidationError); ok {
			errs = append(errs, valErrs)
		} else {
			for _, e := range valErrs.(validator.ValidationErrors) {
				if taskName == "" {
					errs = append(errs, fmt.Errorf(e.Translate(trans)))
				} else {
					errs = append(errs, fmt.Errorf("task '%s': %s", taskName, e.Translate(trans)))
				}
			}
		}
	}
	return errs
}

func initValidator(customValidations []customValidation) error {
	govalidator = validator.New()
	govalidator.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("yaml"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})

	// Register default translators
	translator := en.New()
	uni = ut.New(translator, translator)
	var translatorFound bool
	trans, translatorFound = uni.GetTranslator("en")
	if !translatorFound {
		return fmt.Errorf("failed to initialize validator with translator")
	}
	en_translations.RegisterDefaultTranslations(govalidator, trans)

	// Register Custom validators and translations
	for _, t := range customValidations {
		err := govalidator.RegisterValidationCtx(t.tag, t.validationFn)
		if err != nil {
			return fmt.Errorf("failed to register validation: %s", err.Error())
		}
		err = govalidator.RegisterTranslation(t.tag, trans, registrationFunc(t.tag, t.translation), translateFunc)
		if err != nil {
			return fmt.Errorf("failed to register translations: %s", err.Error())
		}
	}
	return nil
}

// ValidateMountDir verifies that mount values are in proper format <src>:<dest>:<mode>
// Format should match, <mode> is optional which is `readOnly` by default
func ValidateMountDir(ctx context.Context, fl validator.FieldLevel) bool {
	value := fl.Field().String()
	f := func(c rune) bool { return c == ':' }
	mountValues := strings.FieldsFunc(value, f)
	if len(mountValues) != 3 {
		mountValues = append(mountValues, defaultPermissionMode)
	}
	if len(mountValues) != 3 {
		return false
	}
	validPerm := false
	for _, perm := range validDirPermissionModes {
		if mountValues[2] == perm {
			validPerm = true
		}
	}
	return validPerm
}

// ValidateFollowTaskPresent verifies that referenceed task exists
func ValidateFollowTaskPresent(ctx context.Context, fl validator.FieldLevel) bool {
	followTask := strings.TrimSpace(fl.Field().String())
	configs := ctx.Value(configsKey).(*Configs)
	for taskName := range configs.Tasks {
		if taskName == followTask {
			return true
		}
	}
	return false
}

// ParseMountDir verifies that source directory exists and parses the environment variables used in the config
func ParseMountDir(ctx context.Context, fl validator.FieldLevel) bool {
	value := fl.Field().String()
	f := func(c rune) bool { return c == ':' }
	mountValues := strings.FieldsFunc(value, f)
	if len(mountValues) == 0 {
		return false
	}
	parsedDir, err := lookupDirectory(mountValues[0])
	if err != nil {
		return false
	}
	return util.DirExists(parsedDir)
}

// GetConfigs reads and parses tasks from the dunner file
func GetConfigs(filename string) (*Configs, error) {
	fileContents, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Fatal(err)
	}

	var configs Configs
	if err := yaml.Unmarshal(fileContents, &configs.Tasks); err != nil {
		log.Fatal(err)
	}

	loadDotEnv()
	if err := parseEnv(&configs); err != nil {
		log.Fatal(err)
	}

	return &configs, nil
}

func loadDotEnv() {
	file := viper.GetString("DotenvFile")
	var err error
	dotEnv, err = godotenv.Read(file)
	if err != nil {
		log.Warn(err)
	}
}

func parseEnv(configs *Configs) error {
	for k, tasks := range (*configs).Tasks {
		for j, task := range tasks {
			for i, envVar := range task.Envs {
				var str = strings.Split(envVar, "=")
				if len(str) != 2 {
					return fmt.Errorf(
						`config: invalid format of environment variable: %v`,
						envVar,
					)
				}
				var pattern = "^`\\$.+`$"
				check, err := regexp.MatchString(pattern, str[1])
				if err != nil {
					log.Fatal(err)
				}
				if check {
					var key = strings.Replace(
						strings.Replace(
							str[1],
							"`",
							"",
							-1,
						),
						"$",
						"",
						1,
					)
					var val string
					if v, isSet := os.LookupEnv(key); isSet {
						val = v
					}
					if v, isSet := dotEnv[key]; isSet {
						val = v
					}
					if val == "" {
						return fmt.Errorf(
							`config: could not find environment variable '%v' in %s file or among host environment variables`,
							key,
							viper.GetString("DotenvFile"),
						)
					}
					var newEnv = str[0] + "=" + val
					(*configs).Tasks[k][j].Envs[i] = newEnv
				}
			}
		}
	}

	return nil
}

// DecodeMount parses mount format for directories to be mounted as bind volumes
func DecodeMount(mounts []string, step *docker.Step) error {
	for _, m := range mounts {
		arr := strings.Split(
			strings.Trim(strings.Trim(m, `'`), `"`),
			":",
		)
		var readOnly = true
		if len(arr) == 3 {
			if arr[2] == "wr" || arr[2] == "w" {
				readOnly = false
			}
		}
		parsedSrcDir, err := lookupDirectory(arr[0])
		if err != nil {
			return err
		}
		parsedDestDir, err := lookupDirectory(arr[1])
		if err != nil {
			return err
		}
		src, err := filepath.Abs(joinPathRelToHome(parsedSrcDir))
		if err != nil {
			return err
		}

		(*step).ExtMounts = append((*step).ExtMounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   src,
			Target:   parsedDestDir,
			ReadOnly: readOnly,
		})
	}
	return nil
}

// Replaces dir having any environment variables in form `$ENV_NAME` and returns a parsed string
func lookupDirectory(dir string) (string, error) {
	hostDirpattern := "`\\$(?P<name>[^`]+)`"
	hostDirRegex := regexp.MustCompile(hostDirpattern)
	matches := hostDirRegex.FindAllStringSubmatch(dir, -1)

	parsedDir := dir
	for _, matchArr := range matches {
		envKey := matchArr[1]
		var val string
		if v, isSet := os.LookupEnv(envKey); isSet {
			val = v
		}
		if v, isSet := dotEnv[envKey]; isSet {
			val = v
		}
		if val == "" {
			return dir, fmt.Errorf(`Could not find environment variable '%v'`, envKey)
		}
		parsedDir = strings.Replace(parsedDir, fmt.Sprintf("`$%s`", envKey), val, -1)
	}
	return parsedDir, nil
}

func joinPathRelToHome(p string) string {
	if p[0] == '~' {
		return path.Join(util.HomeDir, strings.Trim(p, "~"))
	}
	return p
}

func registrationFunc(tag string, translation string) validator.RegisterTranslationsFunc {
	return func(ut ut.Translator) (err error) {
		if err = ut.Add(tag, translation, true); err != nil {
			return
		}
		return
	}
}

func translateFunc(ut ut.Translator, fe validator.FieldError) string {
	t, err := ut.T(fe.Tag(), reflect.ValueOf(fe.Value()).String(), fe.Param())
	if err != nil {
		return fe.(error).Error()
	}
	return t
}
