import 'dart:convert';

import 'package:http/http.dart' as http;

/// Thrown when the BFF cannot be reached or answers without a screen to show.
class ApiException implements Exception {
  ApiException(this.status, this.message);
  final int status;
  final String message;
  @override
  String toString() => 'ApiException($status): $message';
}

/// What every BFF call resolves to: the screen to render next, plus an
/// optional message to surface (login failure, lockout notice).
class ScreenResult {
  ScreenResult(this.screen, {this.message});
  final Map<String, dynamic> screen;
  final String? message;
}

/// The only network client in the app. It knows the BFF base URL, holds the
/// session token, and hands back screens. It never interprets banking data.
class ApiClient {
  ApiClient({required this.baseUrl, http.Client? client})
      : _client = client ?? http.Client();

  final String baseUrl;
  final http.Client _client;

  // The token lives in memory only. Persisting it, and binding it to the
  // device, is the native security plugin's job (roadmap Phase 2, step 3).
  String? _token;

  bool get isLoggedIn => _token != null;

  Map<String, String> _headers({bool json = false}) => {
        'Accept': 'application/json',
        if (json) 'Content-Type': 'application/json',
        if (_token != null) 'Authorization': 'Bearer $_token',
      };

  /// GET a screen by the path the BFF gave us.
  Future<ScreenResult> getScreen(String path) async {
    final res = await _client.get(Uri.parse('$baseUrl$path'), headers: _headers());
    return _resolve(res);
  }

  /// POST form values to a submit path (login is just a form to the client).
  Future<ScreenResult> submit(String path, Map<String, String> fields) async {
    final res = await _client.post(
      Uri.parse('$baseUrl$path'),
      headers: _headers(json: true),
      body: jsonEncode(fields),
    );
    return _resolve(res);
  }

  /// End the session on the server and forget the token.
  Future<ScreenResult> logout() async {
    try {
      final res = await _client.post(Uri.parse('$baseUrl/v1/logout'), headers: _headers());
      return _resolve(res);
    } finally {
      _token = null;
    }
  }

  ScreenResult _resolve(http.Response res) {
    Map<String, dynamic> body;
    try {
      body = res.body.isEmpty ? <String, dynamic>{} : jsonDecode(res.body) as Map<String, dynamic>;
    } on FormatException {
      throw ApiException(res.statusCode, 'Unexpected reply from server');
    }
    if (body['token'] is String) {
      _token = body['token'] as String;
    }
    if (body['error'] == 'unauthenticated') {
      _token = null;
    }
    final screen = body.containsKey('schema')
        ? body
        : body['screen'] is Map<String, dynamic>
            ? body['screen'] as Map<String, dynamic>
            : null;
    if (screen == null) {
      throw ApiException(res.statusCode, body['message']?.toString() ?? 'HTTP ${res.statusCode}');
    }
    return ScreenResult(screen, message: body['message']?.toString());
  }
}
