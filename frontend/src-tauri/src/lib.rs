#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
  let builder = tauri::Builder::default()
    // The window comes back where and as big as it was left.
    .plugin(tauri_plugin_window_state::Builder::default().build());

  // macOS shows an app menu (SADE, Edit, Window...) in the menu bar; the
  // system's default one gives copy/paste shortcuts and Quit. Windows and
  // Linux windows carry no menu bar.
  #[cfg(target_os = "macos")]
  let builder = builder.menu(|handle| tauri::menu::Menu::default(handle));

  builder
    .setup(|app| {
      if cfg!(debug_assertions) {
        app.handle().plugin(
          tauri_plugin_log::Builder::default()
            .level(log::LevelFilter::Info)
            .build(),
        )?;
      }
      Ok(())
    })
    .run(tauri::generate_context!())
    .expect("error while building tauri application");
}
