# webdav-server

# WebDAV

WebDAV (Web Distributed Authoring and Versioning) is HTTP plus extra methods and rules that turn a web server into a remote file system

On top of plain HTTP (GET, PUT, DELETE), WebDAV adds:

Collections: folders, created with MKCOL
Properties: metadata about files (size, modified date, type), read with PROPFIND and written with PROPPATCH
Copy and move: server-side COPY and MOVE
Locking: LOCK / UNLOCK, so two clients don’t edit the same file at once

# CalDAV
CalDAV is WebDAV with the added following rules: 
- calendar discovery (principal → calendar home → calendars)
- REPORT queries
- iCalendar (.ics) validation

golang.org/x/net/webdav knows none of that, so we’ll use github.com/emersion/go-webdav, a small library that has a caldav package

You only implement a “backend” (where data lives), and the library does the protocol.

``bash
mkdir caldav-server && cd caldav-server
go mod init caldav-server
go get github.com/emersion/go-webdav github.com/emersion/go-ical

# Tutorial using curl

BASE=http://localhost:8080
CAL=$BASE/remote.php/dav/calendars/alice/
AUTH="-u me:secret"

## List calendars
curl $AUTH -X PROPFIND -H "Depth: 1" $BASE/personal/

## Create calendar
curl $AUTH -i -X MKCALENDAR $BASE/remote.php/dav/calendars/alice/work

With our backend, expect a 403/405/501 error

## Delete calendar
curl $AUTH -i -X DELETE $BASE/user/calendars/work/

## Rename calendar properties
curl $AUTH -X PROPPATCH -H "Content-Type: application/xml" $CAL/ --data '<?xml version="1.0"?>
<D:propertyupdate xmlns:D="DAV:">
  <D:set><D:prop><D:displayname>Renamed</D:displayname></D:prop></D:set>
</D:propertyupdate>'


## Save a file as iCalendar
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

curl $AUTH -i -X PUT \
  -H "Content-Type: text/calendar; charset=utf-8" \
  -H "If-None-Match: *" \
  --data-binary @meeting.ics \
  $CAL/meeting-001.ics


## List events
curl $AUTH -X PROPFIND -H "Depth: 1" $CAL/

## Read a single event
curl $AUTH -i $CAL/meeting-001.ics

## Edit an event
ETAG=$(curl $AUTH -sI $CAL/meeting-001.ics | grep -i '^etag:' | cut -d' ' -f2 | tr -d '\r')

sed -i 's/SUMMARY:Team meeting/SUMMARY:Team meeting (moved)/' meeting.ics

## Delete an event
curl $AUTH -i -X DELETE $CAL/meeting-001.ics


## Query events vy date range
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

curl $AUTH -i -X PUT \
  -H "Content-Type: text/calendar; charset=utf-8" \
  -H "If-Match: $ETAG" \
  --data-binary @meeting.ics \
  $CAL/meeting-001.ics


# Tutorial using rclone


## Configure the remote
rclone config create caldav webdav \
  url=http://localhost:8080/user/calendars/default/ \
  vendor=other \
  user=me \
  pass=$(rclone obscure secret)

## Commands
Action	rclone	curl equivalent
List events	rclone lsf caldav:	PROPFIND
List with size/date	rclone lsl caldav:	PROPFIND
Create / upload	rclone copyto meeting.ics caldav:meeting-001.ics	PUT
Edit (overwrite)	rclone copyto meeting.ics caldav:meeting-001.ics	PUT
Read	rclone cat caldav:meeting-001.ics	GET
Download	rclone copyto caldav:meeting-001.ics ./meeting.ics	GET
Delete one event	rclone deletefile caldav:meeting-001.ics	DELETE
Delete all events	rclone delete caldav:	many DELETEs
Backup everything	rclone sync caldav: ./backup	
Restore	rclone copy ./backup caldav:	

Use sync with care: it deletes files at the destination that are missing at the source.

Calendar-level in rclone
bash
rclone mkdir caldav:work   # sends MKCOL: not supported by our server
rclone rmdir caldav:work   # same

MKCOL is plain WebDAV and is different from MKCALENDAR, so even a multi-calendar CalDAV backend wouldn’t create a proper calendar this way.

## Troubleshooting
If uploads fail with PROPPATCH errors, add --no-update-modtime, since rclone tries to set the modification time after upload and our server doesn’t implement it:
bash
  rclone copyto meeting.ics caldav:meeting-001.ics --no-update-modtime
If PUT returns an error about content type, check your system maps .ics to text/calendar, or use curl, which lets you set the header explicitly.
Add -vv --dump headers to rclone to see the raw HTTP exchange

# Run and test
go run . -user me -pass secret

curl -u me:secret -X PROPFIND -H "Depth: 1" http://localhost:8080/remote.php/dav/calendars/


WebDAV vs CalDAV
	WebDAV (x/net/webdav)	CalDAV (go-webdav/caldav)
You provide	A folder (FileSystem)	A Backend with methods like ListCalendars and PutCalendarObject
Content	Any file	Validated iCalendar (.ics)
Discovery	Not needed	principal → home set → calendars
Queries	PROPFIND only	Adds REPORT (date range filters)