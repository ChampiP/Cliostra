package runtime

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
