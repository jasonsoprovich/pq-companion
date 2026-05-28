; PQ Companion — custom NSIS installer hooks
;
; electron-builder's NSIS template only cleans up the install directory
; ($INSTDIR). It leaves behind a few runtime locations:
;   - %AppData%\PQ Companion           (Electron userData — overlayBounds.json,
;                                       overlayLockState.json, Chromium cache)
;   - %LocalAppData%\PQ Companion      (Electron localUserData / GPUCache)
;   - %LocalAppData%\pq-companion-updater (electron-updater download cache)
;   - %UserProfile%\.pq-companion      (user.db, config.yaml, logs/, backups/)
;
; The updater cache is always safe to delete (transient download staging).
; Everything else is real user data and must only be touched on a real
; user-initiated uninstall — NOT during the silent uninstall electron-updater
; runs as part of an auto-update. That distinction is what fixes issue #126:
; previously we used electron-builder's `deleteAppDataOnUninstall: true`, which
; ran unconditionally and wiped overlay positions on every upgrade.

!macro customUnInstall
  ; 1. Always remove the electron-updater download cache. This dir is created
  ;    fresh on every update download; nothing in it is worth keeping.
  RMDir /r "$LOCALAPPDATA\pq-companion-updater"

  ; 2. Real uninstall only — skip everything below when running as part of an
  ;    auto-update (electron-updater invokes the old uninstaller silently
  ;    before installing the new build).
  ${ifNot} ${isUpdated}
    ; %AppData%\PQ Companion — Electron userData. Holds overlayBounds.json,
    ; overlayLockState.json, Chromium Local Storage / IndexedDB / Cache.
    ; Safe to remove on real uninstall; must survive auto-updates.
    RMDir /r "$APPDATA\${PRODUCT_NAME}"
    RMDir /r "$LOCALAPPDATA\${PRODUCT_NAME}"

    ; Offer to remove the user-data directory. Default to "No" so a misclick
    ; on uninstall doesn't nuke triggers, backups, and EQ config snapshots.
    IfFileExists "$PROFILE\.pq-companion\*.*" 0 skip_userdata
      MessageBox MB_YESNO|MB_ICONQUESTION|MB_DEFBUTTON2 \
        "Also remove your PQ Companion settings, triggers, backups, and logs?$\r$\n$\r$\nLocation: $PROFILE\.pq-companion$\r$\n$\r$\nChoose No to keep your data for a future reinstall." \
        /SD IDNO IDNO skip_userdata
      RMDir /r "$PROFILE\.pq-companion"
    skip_userdata:
  ${endIf}
!macroend
