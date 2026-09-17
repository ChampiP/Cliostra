package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Job es el registro durable de un trabajo. Nunca persiste el prompt: solo
// metadatos seguros, estado y resultado/error terminal.
type Job struct {
	ID        string    `json:"id"`
	Adapter   string    `json:"adapter"`
	Repo      string    `json:"repo"`
	ReadOnly  bool      `json:"read_only"`
	Model     string    `json:"model,omitempty"`
	Effort    string    `json:"effort,omitempty"`
	State     State     `json:"state"`
	Reason    string    `json:"reason,omitempty"`
	Result    string    `json:"result,omitempty"`
	Diff      string    `json:"diff,omitempty"`
	Truncated bool      `json:"truncated"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Store persiste trabajos en disco de forma atómica: escribe a un temporal,
// hace fsync del archivo, renombra y hace fsync del directorio contenedor.
// Así una caída a mitad de escritura nunca deja un job.json corrupto.
type Store struct {
	dir string
}

// NewStore crea (si hace falta) el directorio de estado y devuelve un Store
// que persiste ahí.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

func (s *Store) path(id string) string {
	return filepath.Join(s.dir, id+".json")
}

// Save escribe el job de forma atómica.
func (s *Store) Save(j *Job) error {
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	final := s.path(j.ID)
	tmp := final + ".tmp"

	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, final); err != nil {
		return err
	}
	dirF, err := os.Open(s.dir)
	if err != nil {
		return err
	}
	defer dirF.Close()
	return dirF.Sync()
}

// Load lee un job por ID. Devuelve os.ErrNotExist si no existe.
func (s *Store) Load(id string) (*Job, error) {
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		return nil, err
	}
	var j Job
	if err := json.Unmarshal(data, &j); err != nil {
		return nil, err
	}
	return &j, nil
}

// LoadAll lee todos los jobs persistidos, usado en la recuperación al
// reiniciar.
func (s *Store) LoadAll() ([]*Job, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var jobs []*Job
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		id := e.Name()[:len(e.Name())-len(".json")]
		j, err := s.Load(id)
		if err != nil {
			continue
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}
