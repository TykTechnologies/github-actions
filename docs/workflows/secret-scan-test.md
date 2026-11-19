## Secret scan tests

Tests for the secret-scan action. Builds images that leak fake credentials in common ways, plus correctly built controls, and checks every leak is caught, the controls pass, a real multi-platform release image is clean on every platform, and the action fails closed when a scanner cannot run.

Adoption: Internal use for PR workflows on the repository.
