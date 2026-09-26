[English](README.md) · [日本語](README-ja.md) · Deutsch · [Français](README-fr.md)

# dead-mans-ping (`mip`)

[![CI](https://github.com/didvc/dead-mans-ping/actions/workflows/ci.yml/badge.svg)](https://github.com/didvc/dead-mans-ping/actions/workflows/ci.yml)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/didvc/dead-mans-ping)](https://goreportcard.com/report/github.com/didvc/dead-mans-ping)

Ein kleines plattformübergreifendes (Linux, Windows) Go-CLI, das je nach Mausaktivität HTTP-`GET`-Anfragen an einen oder mehrere Endpunkte sendet, zum Beispiel als Totmannschalter, der eine URL anpingt, wenn dein Rechner ein paar Tage unbenutzt war.

![didvc/dead-mans-ping](assets/social-preview.png)

Dazu wird die absolute Cursorposition in einem festen Intervall abgefragt. Das braucht keine erhöhten Rechte und installiert keine globalen Eingabe-Hooks, lässt sich also gefahrlos als normaler Benutzer ausführen.

- Linux: benötigt eine X11-Sitzung (`$DISPLAY`). Funktioniert mit XWayland-Fenstern; natives Wayland gibt die globale Zeigerposition absichtlich nicht preis.
- Windows: nutzt `user32!GetCursorPos` über die Standardbibliothek (kein cgo).

![dead-mans-ping im Einsatz](assets/demo-run.png)

## Installation

```sh
# From source (Go 1.25+); installs the `mip` binary:
go install github.com/didvc/dead-mans-ping/cmd/mip@latest
```

Oder lade ein fertiges Binary für deine Plattform von der Seite [Releases](https://github.com/didvc/dead-mans-ping/releases).

## Bauen

```sh
make            # test + build ./bin/mip for the host
make release    # cross-compile ./bin/mip-linux-amd64 and mip-windows-amd64.exe
make test
```

## Verwendung

```sh
mip --endpoint https://example.com/ping [--endpoint https://backup/ping ...] [options]
```

Alle Anfragen sind HTTP-`GET`. Mindestens ein `--endpoint` ist erforderlich. Endpunkte müssen `http`/`https` mit Host sein; Anfragen haben ein Timeout, begrenzte Weiterleitungen und ein größenbeschränktes Lesen des Bodys.

### Das Verhalten besteht aus drei unabhängigen Entscheidungen

Wann ausgelöst wird (das „Flag“):

| Flag                | Bedeutung                                                           |
| ------------------- | ------------------------------------------------------------------- |
| `--inactive-ping`   | *(Standard)* Flag wird gesetzt, sobald die Maus ≥ `--inactive-period` ruht|
| `--active-ping`     | Flag wird gesetzt, sobald eine Bewegung erkannt wird (`--inactive-period` wird ignoriert) |

Wie gepingt wird, solange das Flag gesetzt ist:

| Flag                | Bedeutung                                                           |
| ------------------- | ------------------------------------------------------------------- |
| `--ping-once`       | *(Standard)* ein Ping pro Flag-Phase                                |
| `--ping-continuous` | Ping alle `--ping-interval`, solange das Flag gesetzt ist; endet, wenn es zurückgesetzt wird |

Lebenszyklus:

| Flag                | Bedeutung                                                           |
| ------------------- | ------------------------------------------------------------------- |
| `--onetime`         | *(Standard)* nach dem ersten Ping bzw. nach Ende der Dauer-Pings beenden |
| `--cold-period D`   | weiterlaufen; Mindestabstand `D` zwischen Pings erzwingen (hat Vorrang vor `--onetime`) |

### Alle Optionen

| Flag                  | Standard | Beschreibung                                    |
| --------------------- | ------- | -------------------------------------------------- |
| `--endpoint URL`         | -                 | Ziel für Aktivitäts-Pings; für mehrere wiederholen |
| `--inactive-period D`    | `3d`              | Ruheschwelle für `--inactive-ping`           |
| `--ping-interval D`      | `30s`             | Wiederholintervall für `--ping-continuous`   |
| `--cold-period D`        | unset             | Mindestabstand zwischen Pings (bedeutet: nicht `--onetime`) |
| `--heartbeat-endpoint URL` | unset           | Lebenszeichen-URL, GET in festem Intervall (siehe unten) |
| `--heartbeat-interval D` | `60s`             | Intervall für `--heartbeat-endpoint`         |
| `--server`               | off               | Steuer-HTTP-Server starten (siehe unten)      |
| `--server-addr HOST:PORT`| `127.0.0.1:8080`  | Lauschadresse für `--server`                  |
| `--poll-interval D`      | `1s`              | Abtastintervall des Cursors                   |
| `--move-threshold N`     | `1.0`             | Mindestabstand in Pixeln, der als Bewegung zählt |
| `--timeout D`            | `10s`             | HTTP-Timeout pro Anfrage                      |
| `--no-log`               | off               | Statuszeile und Ereignisprotokoll abschalten  |

## Heartbeat (Lebenszeichen)

`--heartbeat-endpoint` ist eine eigene Schleife neben den Aktivitäts-Pings: Sie ruft ihre URL unabhängig von der Mausaktivität alle `--heartbeat-interval` per GET ab (sofort beginnend), sodass ein externer Monitor erkennt, dass der Prozess noch läuft. Sie teilt nie Endpunkt oder Timing mit den Aktivitäts-Pings.

```sh
mip --endpoint https://example.com/idle \
    --heartbeat-endpoint https://hc-ping.com/alive --heartbeat-interval 5m
```

## Steuerserver

Mit `--server` wird eine HTTP-Steuerschnittstelle gestartet (standardmäßig an `127.0.0.1:8080` gebunden):

| Endpunkt                    | Wirkung                                                       |
| --------------------------- | ------------------------------------------------------------- |
| `GET /extend?seconds=<N>`   | Inaktivitätsfrist um N Sekunden verschieben (summiert sich)    |
| `GET /extend?until=<unix>`  | Inaktivitätsfrist auf einen absoluten Unix-Zeitstempel setzen  |
| `GET /help`                 | Hilfetext                                                     |

`/extend` verschiebt den Zeitpunkt, an dem ein `--inactive-ping` auslöst: ein „Ich bin noch da“ aus der Ferne, ganz ohne Maus. Die Frist wird nur nach hinten verschoben; gib genau eines von `seconds`/`until` an. Weil sich damit ein Totmannschalter aushebeln lässt, bindet der Server standardmäßig nur an localhost; öffne ihn nur hinter eigener Authentifizierung oder einem Proxy.

```sh
mip --endpoint https://example.com/idle --inactive-period 1h --cold-period 1h --server
# from elsewhere on the box:
curl 'http://127.0.0.1:8080/extend?seconds=3600'   # hold off for another hour
```

![Steuerserver und Heartbeat](assets/demo-server.png)

Zeitangaben akzeptieren `s`, `m`, `h` sowie `d` (Tage) und `w` (Wochen), z. B. `3d`, `1w`, `1d12h`.

### Statuszeile

Sofern `--no-log` nicht angegeben ist, zeigt eine Live-Statuszeile den aktuellen Modus, die Ruhezeit und eine Bewegungsübersicht (gesamte Pixelstrecke und Anzahl der Bewegungen) für die letzte Stunde, den letzten Tag und die letzte Woche:

```
[14:22:07] mode=inactive flag=false idle=1m3s | 1h 4821.5px/142 1d 4821.5px/142 1w 4821.5px/142
```

Die Bewegungsdaten werden in Ein-Minuten-Blöcken in einem Ringpuffer über eine Woche gesammelt, sodass der Speicherbedarf konstant bleibt, egal wie lange der Prozess läuft.

## Beispiele

```sh
# Dead-man's switch: ping once after 3 days idle, then exit (all defaults).
mip --endpoint https://hc-ping.com/UUID

# Heartbeat: while idle ≥ 1h, ping every 5 minutes; keep running,
# no more than one ping per 5 minutes.
mip --inactive-period 1h --ping-continuous --ping-interval 5m \
    --cold-period 5m --endpoint https://example.com/idle

# Presence beacon: ping the instant the mouse moves, at most every 30s.
mip --active-ping --cold-period 30s --endpoint https://example.com/active
```

## CLI-Referenz

Alle Flags (`mip --help`):

![mip --help](assets/demo-help.png)

## Datenschutz

Das Werkzeug liest nur die Bildschirmkoordinaten des Cursors, im Speicher, um Bewegung zu erkennen: keine Tastatureingaben, keine Fenstertitel, keine Bildschirminhalte. Nichts wird auf die Festplatte geschrieben, und es gibt keine Telemetrie. Der einzige Netzwerkverkehr sind die GET-Anfragen, die du über `--endpoint` und `--heartbeat-endpoint` konfigurierst.

## Lizenz

Lizenziert unter der [Apache License 2.0](LICENSE).

<!-- BEGIN gh-mutual-linking -->

---

### Related projects

- [chatnote](https://github.com/didvc/chatnote): Self-hosted note-to-self chatrooms. Privacy-first by design, infinite rooms, Markdown, ephemeral/incognito room types, image uploads, tags, JSON import/export. Astro SSR + SQLite.
- [visited](https://github.com/didvc/visited): Securely collect browsing history over browsers.
<!-- END gh-mutual-linking -->