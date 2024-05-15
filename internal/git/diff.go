package git

import (
	"fmt"
	"os"
	"os/exec"
)

func Diff(s1, s2 string) string {
	if s1 == s2 {
		return ""
	}
	if _, err := exec.LookPath("diff"); err != nil {
		return fmt.Sprintf("diff command unavailable\nold: %q\nnew: %q", s1, s2)
	}
	f1, err := writeTempFile("", "schema_test", []byte(s1+"\n"))
	if err != nil {
		return err.Error()
	}
	defer os.Remove(f1)

	f2, err := writeTempFile("", "schema_test", []byte(s2+"\n"))
	if err != nil {
		return err.Error()
	}
	defer os.Remove(f2)

	cmd := "diff"

	data, err := exec.Command(cmd, "-u", f1, f2).CombinedOutput()
	if len(data) > 0 {
		// diff exits with a non-zero status when the files don't match.
		// Ignore that failure as long as we get output.
		err = nil
	}
	if err != nil {
		data = append(data, []byte(err.Error())...)
	}
	return string(data)
}

func writeTempFile(dir, prefix string, data []byte) (string, error) {
	file, err := os.CreateTemp(dir, prefix)
	if err != nil {
		return "", err
	}
	_, err = file.Write(data)
	if err1 := file.Close(); err == nil {
		err = err1
	}
	if err != nil {
		os.Remove(file.Name())
		return "", err
	}
	return file.Name(), nil
}
