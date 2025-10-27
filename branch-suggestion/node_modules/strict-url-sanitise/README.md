# strict-url-sanitise

Strict URL sanitization with security-focused validation to prevent XSS and command injection attacks.

## Installation

```bash
pnpm add -D strict-url-sanitise
```

## Usage

```typescript
import { sanitizeUrl } from 'strict-url-sanitise'

// Valid URLs are sanitized and returned
const safe = sanitizeUrl('https://example.com/path?param=value')
console.log(safe) // 'https://example.com/path?param=value'

// Invalid URLs throw an error
try {
  sanitizeUrl('javascript:alert("XSS")')
} catch (error) {
  console.log(error.message) // 'Invalid url to pass to open(): javascript:alert("XSS")'
}
```

## Blocked URLs

This library blocks the following types of potentially dangerous URLs:

### Non-HTTP(S) Protocols
```javascript
sanitizeUrl('ftp://example.com/test')                    // ❌ Throws
sanitizeUrl('file:///etc/passwd')                        // ❌ Throws  
sanitizeUrl('javascript:alert("XSS")')                   // ❌ Throws
sanitizeUrl('data:text/html,<script>alert(1)</script>')  // ❌ Throws
```

### Command Injection Attempts
```javascript
sanitizeUrl('https://www.$(calc.exe).com/foo')           // ❌ Throws
sanitizeUrl('javascript:$(cmd /c whoami)')               // ❌ Throws
```

### Invalid Hostnames
```javascript
sanitizeUrl('https://exam ple.com')                      // ❌ Throws (spaces)
```

## What Gets Sanitized

Valid HTTP(S) URLs have their components properly encoded:

```javascript
// Special characters in paths, queries, and fragments are encoded
sanitizeUrl('https://example.com/path with spaces')
// → 'https://example.com/path%2520with%2520spaces'

sanitizeUrl('https://example.com?key=value with spaces')  
// → 'https://example.com/?key=value%20with%20spaces'

// Basic auth credentials are encoded
sanitizeUrl('http://user$(test):pass$(test)@domain.com')
// → 'http://user%24(test):pass%24(test)@domain.com/'
```

This may catch some valid URLs unintentionally but this package is explicitly designed to be as safe as possible to start, then relaxed over time if counter-examples are provided.

## License

MIT
