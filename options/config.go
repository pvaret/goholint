package options

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"github.com/veandco/go-sdl2/sdl"

	"gopkg.in/ini.v1"
)

// Keymap associating an action name (joypad input, UI command...) to an input.
type Keymap map[string]sdl.Keycode

const (
	// ConfigFolder is the path to our dedicated folder in the user's home.
	ConfigFolder = "~/.goholint/"

	// DefaultConfig contains a reasonable default config.ini that's used
	// automatically if no config exists at run time. TODO: embed from file?
	DefaultConfig = `# Most of the flags (except, obviously -config) can be overridden here with
# the exact same name. See -help for details.

#boot = path/to/dmg_rom.bin
#cpuprofile = path/to/cpuprofile.pprof
#level = debug
#fastboot = 1
#nosync = 1
#waitkey = 1
#zoom = 1

# Define your keymap below with <action>=<key>. Key codes are taken from the
# SDL2 documentation (https://wiki.libsdl.org/SDL_Keycode) without the SDLK_
# prefix, and all supported actions are listed hereafter.
[keymap]
up     = UP        # Joypad Up
down   = DOWN      # Joypad Down
left   = LEFT      # Joypad Left
right  = RIGHT     # Joypad Right
a      = s         # A Button
b      = d         # B Button
select = BACKSPACE # Select Button
start  = RETURN    # Start Button

screenshot = F12   # Save a screenshot in the current directory

recordgif = g      # Start/stop recording video output to GIF

# TODO: quit, reset, snapshot...
`
)

// DefaultKeymap is a reasonable default mapping for QWERTY/AZERTY layouts.
var DefaultKeymap = Keymap{
	"up":         sdl.K_UP,
	"down":       sdl.K_DOWN,
	"left":       sdl.K_LEFT,
	"right":      sdl.K_RIGHT,
	"a":          sdl.K_s,
	"b":          sdl.K_d,
	"select":     sdl.K_BACKSPACE,
	"start":      sdl.K_RETURN,
	"screenshot": sdl.K_F12,
	"recordgif":  sdl.K_g,
}

// configKey returns a config key by the given name if it's present in the file
// and not already set by command-line arguments.
func configKey(cfg *ini.File, flags map[string]bool, name string) *ini.Key {
	// FIXME: handle section but so far I only use one for controls.
	if !flags[name] && cfg.Section("").HasKey(name) {
		return cfg.Section("").Key(name)
	}
	return nil
}

// apply a parameter value from the config file to the string variable whose
// address is given, if that parameter was present in the file and not already
// set on the command-line.
func apply(cfg *ini.File, flags map[string]bool, name string, dst *string) {
	if key := configKey(cfg, flags, name); key != nil {
		*dst = key.String()
	}
}

// Same as apply for booleans.
func applyBool(cfg *ini.File, flags map[string]bool, name string, dst *bool) {
	if key := configKey(cfg, flags, name); key != nil {
		if b, err := key.Bool(); err == nil {
			*dst = b
		}
	}
}

// Same as apply for unsigned integers.
func applyUint(cfg *ini.File, flags map[string]bool, name string, dst *uint) {
	if key := configKey(cfg, flags, name); key != nil {
		if i, err := key.Uint(); err == nil {
			*dst = i
		}
	}
}

// Attempt to create home config folder and copy our default config there.
func createDefaultConfig() {
	// Only create default config if the config folder isn't there yet.
	if _, err := os.Stat(ConfigFolder); os.IsNotExist(err) {
		fmt.Println("No config folder. Creating default config now.")

		if err := os.Mkdir(ConfigFolder, 0755); err != nil {
			fmt.Printf("Can't create config folder %s: %v\n", ConfigFolder, err)
			return
		}

		// Create default config.
		path := filepath.Join(ConfigFolder, "config.ini")
		f, err := os.Create(path)
		if err != nil {
			fmt.Printf("Creating %s failed: %v", path, err)
			return
		}
		defer f.Close()

		if _, err := f.WriteString(DefaultConfig); err != nil {
			fmt.Printf("Writing default config failed: %v", err)
		}
	}
}

// Update reads all parameters from a given configuration file and updates the
// Options instance with those values, skipping all options that may already
// have been set on the command-line.
func (o *Options) Update(configPath string, flags map[string]bool) {
	if configPath == "" {
		return
	}

	// Go doesn't natively handle ~ in paths, fair enough.
	if configPath[0] == '~' {
		if u, err := user.Current(); err == nil {
			configPath = filepath.Join(u.HomeDir, configPath[1:])
		}
	}

	cfg, err := ini.Load(configPath)
	if err != nil {
		// No real error handling, this method should be forgiving.
		fmt.Printf("Can't load config file %s (%s)\n", configPath, err)
		return
	}

	// Using quick and dirty helpers because mixed types and lazy.
	apply(cfg, flags, "boot", &o.BootROM)
	apply(cfg, flags, "cpuprofile", &o.CPUProfile)
	// TODO: debug special format.
	apply(cfg, flags, "level", &o.DebugLevel)
	applyBool(cfg, flags, "fastboot", &o.FastBoot)
	applyBool(cfg, flags, "nosync", &o.VSync)
	// TODO: savedir (and just ditch savepath altogether)
	applyBool(cfg, flags, "waitkey", &o.WaitKey)
	applyUint(cfg, flags, "zoom", &o.ZoomFactor)

	// Ignoring options that are not really interesting as a config.
	// Such as -cyles, -gif or -rom...

	// Set keymap here. Build on top of default. TODO: validate.
	keySection := cfg.Section("keymap")
	for key := range o.Keymap {
		// Key() will return the empty string if it doesn't exist, it's fine.
		keyName := keySection.Key(key).String()
		keySym := sdl.GetKeyFromName(keyName)
		if keySym != sdl.K_UNKNOWN {
			o.Keymap[key] = keySym
		}
	}
}
