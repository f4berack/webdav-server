# CalDAV server tutorial (curl and rclone)

Commands for the multi-calendar Go server with a fixed user `alice`.

## Requirements in the code

The tutorial only works if the server has these three things:

1. **URL constants** with the depth the library expects:

   ```go
   const (
   	davRoot       = "/remote.php/dav/"
   	username      = "alice"
   	principalPath = davRoot + username + "/"
   	homePath      = principalPath + "calendars/"
   )
   ```

2. **Handler prefix** in `main.go`:

   ```go
   var dav http.Handler = &caldav.Handler{Backend: b, Prefix: "/remote.php/dav"}
   ```

3. **MKCALENDAR middleware** (`mkcalendarHandler`), because the library answers `405 webdav: unsupported method` to `MKCALENDAR`. Wrap the handler with it before the auth middleware:

   ```go
   dav = b.mkcalendarHandler(dav)
   ```

## Start the server

```bash
go run . -pass secret
```

- The user is fixed to `alice`. Only the `-pass` flag exists (no `-user`).
- Calendars are folders inside `./data`. The default calendar is `personal`, created at startup by `ensureDefault`.

## URL layout

```
/.well-known/caldav                                 -> 301 to /remote.php/dav/
/remote.php/dav/                                    root
/remote.php/dav/alice/                              principal   (depth 1)
/remote.php/dav/alice/calendars/                    calendar home (depth 2)
/remote.php/dav/alice/calendars/personal/           calendar    (depth 3)
/remote.php/dav/alice/calendars/personal/X.ics      event       (depth 4)
```

Collections always end with a trailing `/`. The library compares the principal and home paths exactly, so a missing slash gives `404`.

This is not Nextcloud's exact layout (`principals/users/alice`, `calendars/alice`). The library identifies resources by path depth, and Nextcloud's two trees are both 3 levels deep, so they cannot coexist. Clients follow the URLs the server advertises, so they are not affected.

---

# Tutorial using curl

## Variables

```bash
BASE=http://localhost:8080/remote.php/dav/alice/calendars   # no trailing slash
CAL=$BASE/personal                                          # the default calendar
AUTH="-u alice:secret"
```

## Check discovery, top down

```bash
curl -i http://localhost:8080/.well-known/caldav                                  # 301
curl $AUTH -i -X PROPFIND -H "Depth: 0" http://localhost:8080/remote.php/dav/       # 207 root
curl $AUTH -i -X PROPFIND -H "Depth: 0" http://localhost:8080/remote.php/dav/alice/ # 207 principal
```

If one level fails, that is where the path or the code is wrong.

## List calendars

```bash
curl $AUTH -X PROPFIND -H "Depth: 1" $BASE/
```

## Create a calendar

```bash
curl $AUTH -i -X MKCALENDAR $BASE/work/ \
  -H "Content-Type: application/xml" --data '<?xml version="1.0"?>
<C:mkcalendar xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
  <D:set><D:prop><D:displayname>Work</D:displayname></D:prop></D:set>
</C:mkcalendar>'
```

Expect `201 Created`. Note the trailing slash in the URL. Creating a calendar that already exists answers `405`.

## Delete a calendar

```bash
curl $AUTH -i -X DELETE $BASE/work/
```

This deletes the folder and all its events.

## Rename a calendar

`PROPPATCH` is not implemented (the library answers `501`). To rename, edit the metadata file directly. It is read on every request, so no restart is needed:

```bash
echo '{"name":"Renamed","description":""}' > data/work/.meta.json
```

## Create an event

Save an iCalendar file:

```bash
cat > meeting.ics <<'EOF'
BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//demo//caldav-server//EN
BEGIN:VEVENT
UID:meeting-001@local
DTSTAMP:20261025T100000Z
DTSTART:20261101T090000Z
DTEND:20261101T100000Z
SUMMARY:Team meeting
END:VEVENT
END:VCALENDAR
EOF
```

Upload it:

```bash
curl $AUTH -i -X PUT \
  -H "Content-Type: text/calendar; charset=utf-8" \
  -H "If-None-Match: *" \
  --data-binary @meeting.ics \
  $CAL/meeting-001.ics
```

Expect `201 Created` (the library answers `201` even for updates). The `Content-Type: text/calendar` header is mandatory, otherwise the answer is `400`. Our backend does not enforce `If-None-Match` yet, so an existing event would be overwritten silently.

## List events

```bash
curl $AUTH -X PROPFIND -H "Depth: 1" $CAL/
```

## Read a single event

```bash
curl $AUTH -i $CAL/meeting-001.ics
```

## Edit an event

```bash
# 1. get the current ETag
ETAG=$(curl $AUTH -sI $CAL/meeting-001.ics | grep -i '^etag:' | cut -d' ' -f2 | tr -d '\r')

# 2. change the file (macOS: sed -i '')
sed -i 's/SUMMARY:Team meeting/SUMMARY:Team meeting (moved)/' meeting.ics

# 3. upload it again
curl $AUTH -i -X PUT \
  -H "Content-Type: text/calendar; charset=utf-8" \
  -H "If-Match: $ETAG" \
  --data-binary @meeting.ics \
  $CAL/meeting-001.ics
```

`If-Match` is sent but not yet checked by our server, so it never answers `412 Precondition Failed`. The library passes it to `PutCalendarObject` in `opts.IfMatch`, so the check can be added.

## Query events by date range

```bash
curl $AUTH -X REPORT -H "Depth: 1" -H "Content-Type: application/xml" $CAL/ --data '<?xml version="1.0"?>
<C:calendar-query xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
  <D:prop><D:getetag/><C:calendar-data/></D:prop>
  <C:filter>
    <C:comp-filter name="VCALENDAR">
      <C:comp-filter name="VEVENT">
        <C:time-range start="20261101T000000Z" end="20261130T000000Z"/>
      </C:comp-filter>
    </C:comp-filter>
  </C:filter>
</C:calendar-query>'
```

## Delete an event

```bash
curl $AUTH -i -X DELETE $CAL/meeting-001.ics
```

---

# Tutorial using rclone

rclone sees one calendar as a folder of `.ics` files.

## Configure the remote

```bash
rclone config create caldav webdav \
  url=http://localhost:8080/remote.php/dav/alice/calendars/personal/ \
  vendor=other \
  user=alice \
  pass=$(rclone obscure secret)
```

To use another calendar, create a second remote with that slug in the URL (for example `caldav-work` with `.../alice/calendars/work/`).

## Commands

| Action | rclone | HTTP method |
|---|---|---|
| List events | `rclone lsf caldav:` | `PROPFIND` |
| List with size/date | `rclone lsl caldav:` | `PROPFIND` |
| Create / upload | `rclone copyto meeting.ics caldav:meeting-001.ics` | `PUT` |
| Edit (overwrite) | `rclone copyto meeting.ics caldav:meeting-001.ics` | `PUT` |
| Read | `rclone cat caldav:meeting-001.ics` | `GET` |
| Download | `rclone copyto caldav:meeting-001.ics ./meeting.ics` | `GET` |
| Delete one event | `rclone deletefile caldav:meeting-001.ics` | `DELETE` |
| Delete all events | `rclone delete caldav:` | many `DELETE`s |
| Backup everything | `rclone sync caldav: ./backup` | `GET`s |
| Restore | `rclone copy ./backup caldav:` | `PUT`s |

> **Warning:** `sync` deletes files at the destination that are missing at the source. `rclone sync caldav: ./backup` only changes `./backup`, but reversing the arguments would wipe the calendar.

## Calendar-level operations

Use curl. `rclone mkdir caldav:work` sends `MKCOL`, which is plain WebDAV and different from `MKCALENDAR`, so it does not create a proper calendar through our middleware.

## Troubleshooting

- If uploads fail with `PROPPATCH` errors, rclone is trying to set the modification time, which our server does not implement:

  ```bash
  rclone copyto meeting.ics caldav:meeting-001.ics --no-update-modtime
  ```

- If `PUT` returns a content-type error, use curl, which lets you set the header explicitly.
- Add `-vv --dump headers` to see the raw HTTP exchange.

---

# Run and test

```bash
go run . -pass secret

curl -u alice:secret -X PROPFIND -H "Depth: 1" http://localhost:8080/remote.php/dav/alice/calendars/
```

You should see the `personal` calendar in the XML.

| Status | Likely cause |
|---|---|
| `401` | Wrong user (it must be `alice`) or wrong password |
| `404` | Missing trailing slash, old Nextcloud-style path, or missing `Prefix` in `caldav.Handler` |
| `405` on `MKCALENDAR` with "unsupported method" | `mkcalendarHandler` is not wired in `main.go` |
| `405` on `MKCALENDAR` without that message | The calendar already exists |
| `400` on `PUT` | Missing `Content-Type: text/calendar` or invalid iCalendar body |
| `501` on `PROPPATCH` | Not implemented by the library |