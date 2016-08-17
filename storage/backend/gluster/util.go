package gluster

import (
	"math/rand"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/contiv/errored"
)

// Gluster volume names can onlycontain alphanumeric, '-' and '_' characters.
func (d *Driver) internalName(s string) (string, error) {
	strs := strings.SplitN(s, "/", 2)
	if len(strs) != 2 {
		return "", errored.Errorf("Invalid volume name %q, must be two parts", s)
	}

	pattern := regexp.MustCompile("^[a-zA-Z0-9-]+$")
	if !pattern.MatchString(strs[0]) || !pattern.MatchString(strs[1]) {
		return "", errored.Errorf("Invalid volume name %q: Gluster volume names can onlycontain alphanumeric and '-' characters", s)
	}

	return strings.Join(strs, "_"), nil
}

func (d *Driver) externalName(s string) string {
	return strings.Join(strings.SplitN(s, "_", 2), "/")
}

func unmarshalBricks(glusterBricks []Brick) map[string]interface{} {
	bricks := map[string]interface{}{}
	for _, brick := range glusterBricks {
		temp := strings.Split(brick.Name, ":")
		bricks[temp[0]] = temp[1] // server:brick
	}
	return bricks
}

func marshalBricks(rawBricks map[string]string) (string, error) {
	randStr := getRandomString(5)
	bricks := []string{}
	for server, brick := range rawBricks {
		if isEmpty(server) {
			return "", errored.Errorf("Cannot use empty key as `gluster server` in driver.bricks")
		}

		if isEmpty(brick) {
			return "", errored.Errorf("Cannot use empty value as `gluster brick` in driver.bricks[%q]", server)
		}

		exportDir := server + ":" + strings.TrimSuffix(brick, string(os.PathSeparator))
		bricks = append(bricks, strings.TrimSpace(exportDir)+string(os.PathSeparator)+randStr)
	}

	return strings.Join(bricks, " "), nil
}

func getRandomString(strlen int) string {
	charSet := []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	randStr := make([]byte, 0, strlen)
	rand.Seed(time.Now().UnixNano())
	for i := 0; i < strlen; i++ {
		randStr = append(randStr, charSet[rand.Int()%len(charSet)])
	}
	return string(randStr)
}

func isEmpty(raw string) bool {
	return 0 == len(strings.TrimSpace(raw))
}

func getVolumeServer(rawBricks map[string]string) string {
	for server := range rawBricks {
		return server
	}
	return ""
}
