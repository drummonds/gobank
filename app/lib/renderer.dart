import 'package:flutter/material.dart';

/// Callbacks the renderer raises. The app decides what to do with them; the
/// renderer only turns JSON into widgets.
typedef NavigateFn = void Function(String path);
typedef SubmitFn = void Function(String path, Map<String, String> fields);
typedef LogoutFn = void Function();

/// Highest screen schema this renderer understands. A newer schema still
/// renders; unknown component types fall back to a placeholder.
const int supportedSchema = 1;

const _tones = <String, Color>{
  'savings': Color(0xFF00947E),
  'lending': Color(0xFF3E8ED0),
  'positive': Color(0xFF48C78E),
  'negative': Color(0xFFF14668),
  'danger': Color(0xFFCC0F35),
  'muted': Color(0xFF7A7A7A),
};

Color _tone(String? t, {Color fallback = const Color(0xFF363636)}) =>
    _tones[t ?? ''] ?? fallback;

const _icons = <String, IconData>{
  'home': Icons.home,
  'accounts': Icons.credit_card,
  'activity': Icons.receipt_long,
  'savings': Icons.savings,
  'lending': Icons.account_balance_wallet,
  'in': Icons.arrow_downward,
  'out': Icons.arrow_upward,
  'interest': Icons.star,
  'loan': Icons.account_balance,
  'bank': Icons.account_balance,
};

IconData? _icon(String? name) => _icons[name ?? ''];

String _s(Map<String, dynamic> m, String key) => (m[key] ?? '').toString();

/// Renders one server-driven screen.
class ScreenView extends StatelessWidget {
  const ScreenView({
    super.key,
    required this.screen,
    required this.onNavigate,
    required this.onSubmit,
    required this.onLogout,
    this.busy = false,
  });

  final Map<String, dynamic> screen;
  final NavigateFn onNavigate;
  final SubmitFn onSubmit;
  final LogoutFn onLogout;
  final bool busy;

  void _act(Map<String, dynamic>? action) {
    if (action == null) return;
    if (action['logout'] == true) {
      onLogout();
    } else if (_s(action, 'screen').isNotEmpty) {
      onNavigate(_s(action, 'screen'));
    }
  }

  @override
  Widget build(BuildContext context) {
    final back = screen['back'] as Map<String, dynamic>?;
    final nav = screen['nav'] as Map<String, dynamic>?;
    final body = (screen['body'] as List<dynamic>? ?? const []).cast<Map<String, dynamic>>();

    return Scaffold(
      appBar: AppBar(
        backgroundColor: _tones['savings'],
        foregroundColor: Colors.white,
        leading: back == null
            ? null
            : IconButton(icon: const Icon(Icons.arrow_back_ios_new), onPressed: () => _act(back)),
        title: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(_s(screen, 'title'), style: const TextStyle(fontSize: 18)),
            if (_s(screen, 'subtitle').isNotEmpty)
              Text(_s(screen, 'subtitle'), style: const TextStyle(fontSize: 12, color: Colors.white70)),
          ],
        ),
        bottom: busy
            ? const PreferredSize(preferredSize: Size.fromHeight(2), child: LinearProgressIndicator(minHeight: 2))
            : null,
      ),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [for (final c in body) _component(context, c)],
      ),
      bottomNavigationBar: nav == null ? null : _navBar(nav),
    );
  }

  Widget _navBar(Map<String, dynamic> nav) {
    final tabs = (nav['tabs'] as List<dynamic>? ?? const []).cast<Map<String, dynamic>>();
    if (tabs.length < 2) return const SizedBox.shrink();
    final active = tabs.indexWhere((t) => _s(t, 'key') == _s(nav, 'active'));
    return NavigationBar(
      selectedIndex: active < 0 ? 0 : active,
      onDestinationSelected: (i) => onNavigate(_s(tabs[i], 'screen')),
      destinations: [
        for (final t in tabs)
          NavigationDestination(icon: Icon(_icon(_s(t, 'icon')) ?? Icons.circle), label: _s(t, 'label')),
      ],
    );
  }

  Widget _component(BuildContext context, Map<String, dynamic> c) {
    switch (_s(c, 'type')) {
      case 'hero':
        return _hero(c);
      case 'row':
        return _row(c);
      case 'tx':
        return _tx(c);
      case 'heading':
        return Padding(
          padding: const EdgeInsets.only(top: 12, bottom: 4),
          child: Text(_s(c, 'text'),
              style: TextStyle(fontSize: 12, fontWeight: FontWeight.bold, color: _tones['muted'])),
        );
      case 'text':
        return _text(c);
      case 'details':
        return _details(c);
      case 'form':
        return _FormComponent(component: c, onSubmit: onSubmit, enabled: !busy);
      case 'button':
        return _button(c);
      default:
        // Unknown component: show something rather than nothing, so an older
        // app keeps working against a newer BFF.
        return Padding(
          padding: const EdgeInsets.symmetric(vertical: 8),
          child: Text('[${_s(c, 'type')}]', textAlign: TextAlign.center, style: TextStyle(color: _tones['muted'])),
        );
    }
  }

  Widget _hero(Map<String, dynamic> c) {
    final base = _tone(_s(c, 'tone'), fallback: _tones['savings']!);
    final pairs = (c['pairs'] as List<dynamic>? ?? const []).cast<Map<String, dynamic>>();
    return Container(
      margin: const EdgeInsets.only(bottom: 16),
      padding: const EdgeInsets.all(20),
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(14),
        gradient: LinearGradient(colors: [base, Color.lerp(base, Colors.white, 0.15)!]),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(_s(c, 'title'), style: const TextStyle(color: Colors.white70, fontSize: 13)),
          Text(_s(c, 'value'),
              style: const TextStyle(color: Colors.white, fontSize: 30, fontWeight: FontWeight.bold)),
          if (pairs.isNotEmpty)
            Padding(
              padding: const EdgeInsets.only(top: 8),
              child: Row(
                mainAxisAlignment: MainAxisAlignment.spaceBetween,
                children: [
                  for (final p in pairs)
                    Text('${_s(p, 'label')}: ${_s(p, 'value')}',
                        style: const TextStyle(color: Colors.white70, fontSize: 12)),
                ],
              ),
            ),
        ],
      ),
    );
  }

  Widget _row(Map<String, dynamic> c) {
    final action = c['action'] as Map<String, dynamic>?;
    return Card(
      margin: const EdgeInsets.only(bottom: 8),
      elevation: 0,
      color: const Color(0xFFFAFAFA),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(10),
        side: BorderSide(color: _tone(_s(c, 'tone')), width: 0),
      ),
      child: Container(
        decoration: BoxDecoration(
          border: Border(left: BorderSide(color: _tone(_s(c, 'tone')), width: 4)),
          borderRadius: BorderRadius.circular(10),
        ),
        child: ListTile(
          leading: Icon(_icon(_s(c, 'icon')), color: _tone(_s(c, 'tone'))),
          title: Text(_s(c, 'title'), style: const TextStyle(fontWeight: FontWeight.w600)),
          subtitle: Text(_s(c, 'subtitle')),
          trailing: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            crossAxisAlignment: CrossAxisAlignment.end,
            children: [
              Text(_s(c, 'value'), style: const TextStyle(fontWeight: FontWeight.bold)),
              Text(_s(c, 'note'), style: TextStyle(fontSize: 11, color: _tones['muted'])),
            ],
          ),
          onTap: action == null ? null : () => _act(action),
        ),
      ),
    );
  }

  Widget _tx(Map<String, dynamic> c) {
    return ListTile(
      contentPadding: EdgeInsets.zero,
      leading: CircleAvatar(
        backgroundColor: const Color(0xFFF0F0F0),
        child: Icon(_icon(_s(c, 'icon')), size: 18, color: Colors.black54),
      ),
      title: Text(_s(c, 'title'), style: const TextStyle(fontSize: 14, fontWeight: FontWeight.w600)),
      subtitle: Text(_s(c, 'subtitle'), style: const TextStyle(fontSize: 12)),
      trailing: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        crossAxisAlignment: CrossAxisAlignment.end,
        children: [
          Text(_s(c, 'value'),
              style: TextStyle(fontWeight: FontWeight.bold, color: _tone(_s(c, 'tone')))),
          Text(_s(c, 'note'), style: TextStyle(fontSize: 11, color: _tones['muted'])),
        ],
      ),
    );
  }

  Widget _text(Map<String, dynamic> c) {
    switch (_s(c, 'tone')) {
      case 'danger':
        return Container(
          margin: const EdgeInsets.only(bottom: 12),
          padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
          decoration: BoxDecoration(color: const Color(0xFFFEECF0), borderRadius: BorderRadius.circular(10)),
          child: Text(_s(c, 'text'), style: TextStyle(color: _tones['danger'], fontSize: 13)),
        );
      case 'muted':
        return Padding(
          padding: const EdgeInsets.symmetric(vertical: 16),
          child: Text(_s(c, 'text'), textAlign: TextAlign.center, style: TextStyle(fontSize: 12, color: _tones['muted'])),
        );
      default:
        return Padding(padding: const EdgeInsets.symmetric(vertical: 8), child: Text(_s(c, 'text')));
    }
  }

  Widget _details(Map<String, dynamic> c) {
    final pairs = (c['pairs'] as List<dynamic>? ?? const []).cast<Map<String, dynamic>>();
    return Container(
      margin: const EdgeInsets.only(bottom: 16),
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(color: const Color(0xFFFAFAFA), borderRadius: BorderRadius.circular(10)),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(_s(c, 'title'), style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 14)),
          for (final p in pairs)
            Padding(
              padding: const EdgeInsets.symmetric(vertical: 6),
              child: Row(
                mainAxisAlignment: MainAxisAlignment.spaceBetween,
                children: [
                  Text(_s(p, 'label'), style: TextStyle(fontSize: 13, color: _tones['muted'])),
                  Text(_s(p, 'value'), style: const TextStyle(fontSize: 13)),
                ],
              ),
            ),
        ],
      ),
    );
  }

  Widget _button(Map<String, dynamic> c) {
    final action = c['action'] as Map<String, dynamic>?;
    final muted = _s(c, 'tone') == 'muted';
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 8),
      child: muted
          ? TextButton(onPressed: busy ? null : () => _act(action), child: Text(_s(c, 'title')))
          : FilledButton(onPressed: busy ? null : () => _act(action), child: Text(_s(c, 'title'))),
    );
  }
}

class _FormComponent extends StatefulWidget {
  const _FormComponent({required this.component, required this.onSubmit, required this.enabled});
  final Map<String, dynamic> component;
  final SubmitFn onSubmit;
  final bool enabled;

  @override
  State<_FormComponent> createState() => _FormComponentState();
}

class _FormComponentState extends State<_FormComponent> {
  final _formKey = GlobalKey<FormState>();
  final _controllers = <String, TextEditingController>{};

  List<Map<String, dynamic>> get _fields =>
      (widget.component['fields'] as List<dynamic>? ?? const []).cast<Map<String, dynamic>>();

  @override
  void dispose() {
    for (final c in _controllers.values) {
      c.dispose();
    }
    super.dispose();
  }

  void _submit() {
    if (!(_formKey.currentState?.validate() ?? false)) return;
    final action = widget.component['action'] as Map<String, dynamic>?;
    final path = _s(action ?? const {}, 'submit');
    if (path.isEmpty) return;
    widget.onSubmit(path, {for (final e in _controllers.entries) e.key: e.value.text});
  }

  @override
  Widget build(BuildContext context) {
    return Form(
      key: _formKey,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          for (final f in _fields) ...[
            TextFormField(
              controller: _controllers.putIfAbsent(_s(f, 'name'), TextEditingController.new),
              enabled: widget.enabled,
              obscureText: _s(f, 'kind') == 'password',
              keyboardType: _s(f, 'kind') == 'number' ? TextInputType.number : TextInputType.text,
              autocorrect: false,
              decoration: InputDecoration(
                labelText: _s(f, 'label'),
                hintText: _s(f, 'placeholder'),
                border: OutlineInputBorder(borderRadius: BorderRadius.circular(10)),
              ),
              validator: (v) =>
                  f['required'] == true && (v == null || v.isEmpty) ? '${_s(f, 'label')} is required' : null,
              onFieldSubmitted: (_) => _submit(),
            ),
            const SizedBox(height: 12),
          ],
          FilledButton(
            onPressed: widget.enabled ? _submit : null,
            child: Text(_s(widget.component, 'title')),
          ),
        ],
      ),
    );
  }
}
