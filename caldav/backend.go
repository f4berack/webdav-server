package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/caldav"
)

const (
	davRoot       = "/remote.php/dav/"
	username      = "alice"
	principalPath = davRoot + username + "/"
	homePath      = principalPath + "calendars/"
)

type backend struct {
	dir string
}

func httpErr(code int, msg string) error {
	return webdav.NewHTTPError(code, fmt.Errorf("%s", msg))
}

func (b *backend) loadCalendar(slug string) (*caldav.Calendar, error) {
	info, err := os.Stat(b.calDir(slug))
	if os.IsNotExist(err) || (err == nil && !info.IsDir()) {
		return nil, httpErr(http.StatusNotFound, "no such calendar")
	}
	if err != nil {
		return nil, err
	}

	m := meta{Name: slug}
	if data, err := os.ReadFile(filepath.Join(b.calDir(slug), ".meta.json")); err == nil {
		_ = json.Unmarshal(data, &m)
		if m.Name == "" {
			m.Name = slug
		}
	}

	return &caldav.Calendar{
		Path:                  calendarURL(slug),
		Name:                  m.Name,
		Description:           m.Description,
		SupportedComponentSet: []string{ical.CompEvent, ical.CompToDo},
	}, nil
}

// parsePath splits a URL into calendar slug and event file name.
//
//	/.../alice/work/       -> "work", ""
//	/.../alice/work/a.ics  -> "work", "a.ics"
func parsePath(urlPath string) (slug, event string, err error) {
	if !strings.HasPrefix(urlPath, homePath) {
		return "", "", httpErr(http.StatusNotFound, "not found")
	}
	rest := strings.Trim(strings.TrimPrefix(urlPath, homePath), "/")
	slug, event, _ = strings.Cut(rest, "/")
	if slug == "" {
		return "", "", httpErr(http.StatusNotFound, "not found")
	}
	return slug, event, nil
}

// ---------- reading ----------

func (b *backend) readObject(urlPath string) (*caldav.CalendarObject, error) {
	slug, event, err := parsePath(urlPath)
	if err != nil {
		return nil, err
	}
	if event == "" {
		return nil, httpErr(http.StatusNotFound, "not an event")
	}
	file := filepath.Join(b.calDir(slug), event)

	raw, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return nil, webdav.NewHTTPError(http.StatusNotFound, err)
	}
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(file)
	if err != nil {
		return nil, err
	}

	data, err := ical.NewDecoder(bytes.NewReader(raw)).Decode()
	if err != nil {
		return nil, err
	}

	sum := sha256.Sum256(raw)
	return &caldav.CalendarObject{
		Path:          urlPath,
		ModTime:       info.ModTime(),
		ContentLength: int64(len(raw)),
		ETag:          fmt.Sprintf("%x", sum[:16]),
		Data:          data,
	}, nil
}

func calendarURL(slug string) string { return homePath + slug + "/" }

func (b *backend) calDir(slug string) string { return filepath.Join(b.dir, slug) }

func (b *backend) CurrentUserPrincipal(_ context.Context) (string, error) {
	return principalPath, nil
}

func (b *backend) CalendarHomeSetPath(_ context.Context) (string, error) {
	return homePath, nil
}

// mkcal creates a calendar folder and returns the HTTP status to answer with.
func (b *backend) mkcal(path, name, desc string) (int, error) {
	slug, event, err := parsePath(path)
	if err != nil {
		return http.StatusNotFound, err
	}
	if event != "" {
		return http.StatusForbidden, fmt.Errorf("calendars cannot be nested")
	}
	if err := os.MkdirAll(b.dir, 0o755); err != nil {
		return http.StatusInternalServerError, err
	}
	if err := os.Mkdir(b.calDir(slug), 0o755); err != nil {
		if os.IsExist(err) {
			return http.StatusMethodNotAllowed, fmt.Errorf("calendar already exists")
		}
		return http.StatusInternalServerError, err
	}
	if name == "" {
		name = slug
	}
	if err := b.writeMeta(slug, meta{Name: name, Description: desc}); err != nil {
		return http.StatusInternalServerError, err
	}
	return http.StatusCreated, nil
}

func (b *backend) CreateCalendar(_ context.Context, cal *caldav.Calendar) error {
	code, err := b.mkcal(cal.Path, cal.Name, cal.Description)
	if err != nil {
		return webdav.NewHTTPError(code, err)
	}
	return nil
}

func (b *backend) ListCalendars(_ context.Context) ([]caldav.Calendar, error) {
	entries, err := os.ReadDir(b.dir)
	if err != nil {
		return nil, err
	}

	var calendars []caldav.Calendar
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		cal, err := b.loadCalendar(e.Name())
		if err != nil {
			log.Printf("skipping calendar %s: %v", e.Name(), err)
			continue
		}
		calendars = append(calendars, *cal)
	}
	return calendars, nil
}

func (b *backend) GetCalendar(_ context.Context, path string) (*caldav.Calendar, error) {
	slug, event, err := parsePath(path)
	if err != nil {
		return nil, err
	}
	if event != "" {
		return nil, httpErr(http.StatusNotFound, "not a calendar")
	}
	return b.loadCalendar(slug)
}

func (b *backend) GetCalendarObject(_ context.Context, path string, _ *caldav.CalendarCompRequest) (*caldav.CalendarObject, error) {
	return b.readObject(path)
}

func (b *backend) ListCalendarObjects(_ context.Context, path string, _ *caldav.CalendarCompRequest) ([]caldav.CalendarObject, error) {
	slug, _, err := parsePath(path)
	if err != nil {
		return nil, err
	}
	if _, err := b.loadCalendar(slug); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(b.calDir(slug))
	if err != nil {
		return nil, err
	}

	var objects []caldav.CalendarObject
	for _, e := range entries {
		// only .ics files: this skips .meta.json and *.tmp
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".ics") {
			continue
		}
		obj, err := b.readObject(calendarURL(slug) + e.Name())
		if err != nil {
			log.Printf("skipping %s/%s: %v", slug, e.Name(), err)
			continue
		}
		objects = append(objects, *obj)
	}
	return objects, nil
}

func (b *backend) QueryCalendarObjects(ctx context.Context, path string, query *caldav.CalendarQuery) ([]caldav.CalendarObject, error) {
	all, err := b.ListCalendarObjects(ctx, path, &query.CompRequest)
	if err != nil {
		return nil, err
	}
	return caldav.Filter(query, all)
}

// ---------- writing ----------

func (b *backend) PutCalendarObject(_ context.Context, path string, cal *ical.Calendar, _ *caldav.PutCalendarObjectOptions) (*caldav.CalendarObject, error) {
	slug, event, err := parsePath(path)
	if err != nil {
		return nil, err
	}
	if event == "" {
		return nil, httpErr(http.StatusBadRequest, "missing event name")
	}
	if _, err := b.loadCalendar(slug); err != nil {
		return nil, err
	}

	// write to a temp file, then rename: an event is never half-written
	tmp, err := os.CreateTemp(b.calDir(slug), "*.tmp")
	if err != nil {
		return nil, err
	}
	if err := ical.NewEncoder(tmp).Encode(cal); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return nil, err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(b.calDir(slug), event)); err != nil {
		os.Remove(tmp.Name())
		return nil, err
	}
	return b.readObject(path)
}

// DeleteCalendarObject handles DELETE for events AND whole calendars.
func (b *backend) DeleteCalendarObject(_ context.Context, path string) error {
	slug, event, err := parsePath(path)
	if err != nil {
		return err
	}

	if event == "" {
		if _, err := b.loadCalendar(slug); err != nil {
			return err
		}
		return os.RemoveAll(b.calDir(slug))
	}

	err = os.Remove(filepath.Join(b.calDir(slug), event))
	if os.IsNotExist(err) {
		return webdav.NewHTTPError(http.StatusNotFound, err)
	}
	return err
}
