package runtime

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// gitToplevel devuelve la raíz Git absoluta de repo, o error si repo no es
// una raíz Git válida.
func gitToplevel(repo string) (string, error) {
	out, err := runGit(repo, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("repo no es una raíz git: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// gitHeadOID devuelve el OID de HEAD, o error si no hay commit.
func gitHeadOID(repo string) (string, error) {
	out, err := runGit(repo, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("repo sin HEAD: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// gitWorktreeAdd crea un worktree en dest, en detached HEAD sobre oid.
func gitWorktreeAdd(repo, dest, oid string) error {
	_, err := runGit(repo, "worktree", "add", "--detach", dest, oid)
	return err
}

// agentScaffoldingPaths son rutas que los CLIs de agente generan por su
// configuración global al arrancar en cualquier directorio, no por la tarea
// delegada. Ensucian el diff que el usuario tiene que revisar antes de
// aplicar el cambio, así que se excluyen. Deliberadamente corta: excluir de
// más escondería cambios reales, que es peor que un poco de ruido. Los
// archivos que el repo ya ignora no hacen falta acá: `git add -A` respeta
// .gitignore.
var agentScaffoldingPaths = []string{".atl"}

// excludePathspecs arma los pathspec mágicos de exclusión de git. Se aplican
// solo al momento de armar el diff: no se toca el .gitignore del repo del
// usuario ni la configuración de git.
func excludePathspecs() []string {
	specs := []string{"--", "."}
	for _, p := range agentScaffoldingPaths {
		specs = append(specs, ":(exclude)"+p)
	}
	return specs
}

// gitDiff devuelve el diff de cambios sin commitear en el worktree (contra
// el OID detached en el que se creó), incluidos archivos nuevos, truncado a
// MaxResultSize para no exceder el límite de resultado. El worktree es
// desechable, así que el `add` interno no tiene efecto fuera de él.
func gitDiff(worktreeDir string) (string, error) {
	if _, err := runGit(worktreeDir, append([]string{"add", "-A"}, excludePathspecs()...)...); err != nil {
		return "", err
	}
	out, err := runGit(worktreeDir, append([]string{"diff", "--cached", "HEAD"}, excludePathspecs()...)...)
	if err != nil {
		return "", err
	}
	if len(out) > MaxResultSize {
		out = out[:MaxResultSize]
	}
	return out, nil
}

// gitWorktreeRemove elimina el worktree administrado en dest.
func gitWorktreeRemove(repo, dest string) error {
	_, err := runGit(repo, "worktree", "remove", "--force", dest)
	return err
}

func runGit(repo string, args ...string) (string, error) {
	full := append([]string{"-C", repo}, args...)
	cmd := exec.Command("git", full...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%v: %s", err, stderr.String())
	}
	return stdout.String(), nil
}

// stripWrite retira los bits de escritura de todo el árbol en root: es una
// barrera operativa, no un sandbox de seguridad.
func stripWrite(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return os.Chmod(path, 0o555)
		}
		return os.Chmod(path, 0o444)
	})
}

// restoreWrite devuelve los bits de escritura dentro de la raíz administrada
// para poder eliminar el worktree al finalizar.
func restoreWrite(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return os.Chmod(path, 0o755)
		}
		return os.Chmod(path, 0o644)
	})
}
