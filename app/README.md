# Model Bank app (Flutter thin shell)

The iOS and Android client for the Model Bank. It renders screens served by
the Go backend-for-frontend (`cmd/bff`) and holds no banking logic: every
amount, colour, icon and navigation target comes from the server as JSON
(see the `screen` package for the schema).

```
lib/main.dart      app shell: one current screen, replaced by each BFF reply
lib/api.dart       the only network client; holds the session token in memory
lib/renderer.dart  JSON screen tree -> widgets; unknown components render a placeholder
```

## Setup

The platform folders (`android/`, `ios/`) are generated, not committed.
With the Flutter SDK installed:

```sh
cd app
flutter create --org uk.bytestone --project-name model_bank_app --platforms=android,ios .
flutter pub get
```

## Run against a local BFF

```sh
# terminal 1, repo root
go run ./cmd/bff            # listens on :8090 with the stub bank

# terminal 2
cd app
flutter run                                   # Android emulator: 10.0.2.2 reaches the host
flutter run --dart-define=BFF_URL=http://192.168.1.10:8090   # a real device on your LAN
```

Stub credentials: customer `cust-001` or `cust-002`, password `password`.

## What is deliberately missing

- **Token persistence.** The session token is held in memory and lost when
  the app is killed. Persisting it, bound to a biometric key, is the native
  security plugin's job (ROADMAP Phase 2, step 3), not a pub.dev package's.
- **Certificate pinning, attestation, screenshot blocking.** Same plugin.
- **Any banking rule.** If you find yourself adding one here, it belongs in
  `bff/screens.go`.
