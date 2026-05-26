; PQ Companion — custom NSIS installer hooks
;
; electron-builder's NSIS template only cleans up the install directory
; ($INSTDIR). It leaves behind two other locations that the app and the
; auto-updater create at runtime:
;   - %LocalAppData%\pq-companion-updater  (electron-updater download cache)
;   - %UserProfile%\.pq-companion          (user.db, config.yaml, logs/, backups/)
;
; The updater cache is always safe to delete (transient download staging).
; The .pq-companion dir contains user data, so we ask before removing it.
;
; Roaming\PQ Companion (Electron userData) is handled by the built-in
; `deleteAppDataOnUninstall: true` flag in electron-builder.yml.

!macro customUnInstall
  ; 1. Always remove the electron-updater download cache. This dir is created
  ;    fresh on every update download; nothing in it is worth keeping.
  RMDir /r "$LOCALAPPDATA\pq-companion-updater"

  ; 2. Offer to remove the user-data directory. Default to "No" so a misclick
  ;    on uninstall doesn't nuke triggers, backups, and EQ config snapshots.
  IfFileExists "$PROFILE\.pq-companion\*.*" 0 skip_userdata
    MessageBox MB_YESNO|MB_ICONQUESTION|MB_DEFBUTTON2 \
      "Also remove your PQ Companion settings, triggers, backups, and logs?$\r$\n$\r$\nLocation: $PROFILE\.pq-companion$\r$\n$\r$\nChoose No to keep your data for a future reinstall." \
      /SD IDNO IDNO skip_userdata
    RMDir /r "$PROFILE\.pq-companion"
  skip_userdata:
!macroend
