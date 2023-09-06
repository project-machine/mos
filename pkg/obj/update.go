package obj

import (
	"fmt"

	"github.com/project-machine/mos/pkg/run"
)

type SectionInput struct {
	Name      string
	VMA       int
	Alignment int
	Path      string
}

func (s *SectionInput) setArgs() []string {
	args := []string{}
	if s.Path != "" {
		args = append(args,
			"--remove-section="+s.Name,
			"--add-section="+s.Name+"="+s.Path)
	}

	if s.VMA != 0 {
		args = append(args, fmt.Sprintf("--change-section-vma=%s=0x%x", s.Name, s.VMA))
	}
	if s.Alignment != 0 {
		args = append(args, fmt.Sprintf("--set-section-alignment=%s=%d", s.Name, s.Alignment))
	}

	return args
}

func SetSections(objpath string, sections ...SectionInput) error {
	cmd := []string{"objcopy"}
	for _, s := range sections {
		cmd = append(cmd, s.setArgs()...)
	}
	cmd = append(cmd, objpath)

	return run.Capture(cmd...).Error()
}
