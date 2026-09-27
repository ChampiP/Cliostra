package runtime

import (
	"log"
	"os"
	"path/filepath"
)

// sweepOrphanWorktrees borra todo lo que haya quedado bajo worktreeRoot al
// arrancar el daemon. Se llama antes de encolar workers, justo después de
// recover() marcar como failed cualquier trabajo no terminal: para ese
// momento ningún worktree residual pertenece a un trabajo en curso (una
// caída del daemon a mitad de execute() salta el defer de cleanupWorktree y
// deja el directorio, con o sin metadata git válida, huérfano).
func sweepOrphanWorktrees(worktreeRoot string) {
	entries, err := os.ReadDir(worktreeRoot)
	if err != nil {
		return
	}
	for _, e := range entries {
		path := filepath.Join(worktreeRoot, e.Name())
		if err := os.RemoveAll(path); err != nil {
			log.Printf("runtime: no se pudo limpiar worktree huérfano %s: %v", path, err)
		}
	}
}

// prepareWorktree crea el worktree administrado sobre el HEAD actual del
// repo y, en modo solo lectura, le quita permiso de escritura.
func prepareWorktree(repo, dest string, readOnly bool) (string, string, error) {
	root, err := gitToplevel(repo)
	if err != nil {
		return "", "", err
	}
	oid, err := gitHeadOID(root)
	if err != nil {
		return "", "", err
	}
	if err := gitWorktreeAdd(root, dest, oid); err != nil {
		return "", "", err
	}
	if readOnly {
		if err := stripWrite(dest); err != nil {
			return "", "", err
		}
	}
	return root, oid, nil
}

// cleanupWorktree restaura permisos y elimina el worktree administrado.
func cleanupWorktree(repo, dest string) {
	_ = restoreWrite(dest)
	_ = gitWorktreeRemove(repo, dest)
}
