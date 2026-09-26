import 'package:flutter/material.dart';

import 'api.dart';
import 'renderer.dart';

/// BFF base URL. Override at build time:
///   flutter run --dart-define=BFF_URL=http://192.168.1.10:8090
/// The default reaches a BFF on the host machine from the Android emulator.
const bffUrl = String.fromEnvironment('BFF_URL', defaultValue: 'http://10.0.2.2:8090');

void main() => runApp(const ModelBankApp());

class ModelBankApp extends StatelessWidget {
  const ModelBankApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Model Bank',
      theme: ThemeData(
        colorSchemeSeed: const Color(0xFF00947E),
        useMaterial3: true,
        scaffoldBackgroundColor: Colors.white,
      ),
      home: const Shell(),
    );
  }
}

/// The whole app: one current screen, fetched from and replaced by the BFF.
class Shell extends StatefulWidget {
  const Shell({super.key});

  @override
  State<Shell> createState() => _ShellState();
}

class _ShellState extends State<Shell> {
  final _api = ApiClient(baseUrl: bffUrl);
  Map<String, dynamic>? _screen;
  bool _busy = false;
  String? _fatal;

  @override
  void initState() {
    super.initState();
    _run(() => _api.getScreen('/v1/screen/login'));
  }

  Future<void> _run(Future<ScreenResult> Function() call) async {
    setState(() => _busy = true);
    try {
      final result = await call();
      if (!mounted) return;
      setState(() {
        _screen = result.screen;
        _fatal = null;
      });
      if (result.message != null && result.message!.isNotEmpty) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(result.message!)));
      }
    } on ApiException catch (e) {
      if (!mounted) return;
      if (_screen == null) {
        setState(() => _fatal = e.message);
      } else {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.message)));
      }
    } catch (e) {
      if (!mounted) return;
      setState(() => _fatal = 'Cannot reach the bank. Check your connection.');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final screen = _screen;
    if (screen == null) {
      return Scaffold(
        body: Center(
          child: _fatal == null
              ? const CircularProgressIndicator()
              : Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Text(_fatal!, textAlign: TextAlign.center),
                    const SizedBox(height: 12),
                    FilledButton(
                      onPressed: () => _run(() => _api.getScreen('/v1/screen/login')),
                      child: const Text('Retry'),
                    ),
                  ],
                ),
        ),
      );
    }
    return PopScope(
      canPop: screen['back'] == null,
      onPopInvokedWithResult: (didPop, _) {
        if (didPop) return;
        final back = screen['back'] as Map<String, dynamic>?;
        final path = back?['screen']?.toString();
        if (path != null && path.isNotEmpty) _run(() => _api.getScreen(path));
      },
      child: ScreenView(
        screen: screen,
        busy: _busy,
        onNavigate: (path) => _run(() => _api.getScreen(path)),
        onSubmit: (path, fields) => _run(() => _api.submit(path, fields)),
        onLogout: () => _run(_api.logout),
      ),
    );
  }
}
