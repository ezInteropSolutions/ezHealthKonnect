// invite.js — Invitation acceptance page logic

(function () {
    'use strict';

    const token = new URLSearchParams(window.location.search).get('token');

    function show(id) {
        ['loading-view', 'expired-view', 'form-view', 'success-view'].forEach(v => {
            const el = document.getElementById(v);
            if (el) el.style.display = v === id ? 'block' : 'none';
        });
    }

    function showAlert(msg, type = 'error') {
        const area = document.getElementById('alert-area');
        if (!area) return;
        area.innerHTML = `<div class="alert alert-${type}">${msg.replace(/</g, '&lt;')}</div>`;
    }

    // Password strength
    function pwStrength(pw) {
        if (!pw) return { score: 0, label: '', cls: '' };
        let score = 0;
        if (pw.length >= 8) score++;
        if (pw.length >= 12) score++;
        if (/[A-Z]/.test(pw)) score++;
        if (/[0-9]/.test(pw)) score++;
        if (/[^A-Za-z0-9]/.test(pw)) score++;
        const labels = ['', 'Very Weak', 'Weak', 'Fair', 'Strong', 'Very Strong'];
        const classes = ['', 'pw-very-weak', 'pw-weak', 'pw-fair', 'pw-strong', 'pw-very-strong'];
        return { score, label: labels[score] || '', cls: classes[score] || '' };
    }

    const pwInput = document.getElementById('password');
    const pwBar = document.getElementById('pw-strength');
    if (pwInput && pwBar) {
        pwInput.addEventListener('input', () => {
            const { score, label } = pwStrength(pwInput.value);
            pwBar.innerHTML = pwInput.value
                ? `<div class="pw-bar"><div class="pw-fill pw-fill--${score}" style="width:${score * 20}%"></div></div><span>${label}</span>`
                : '';
        });
    }

    // Form submit
    const form = document.getElementById('invite-form');
    if (form) {
        form.addEventListener('submit', async e => {
            e.preventDefault();
            const firstName = document.getElementById('firstName').value.trim();
            const lastName = document.getElementById('lastName').value.trim();
            const password = document.getElementById('password').value;
            const confirm = document.getElementById('confirmPassword').value;

            if (!firstName) { showAlert('First name is required'); return; }
            if (password.length < 8) { showAlert('Password must be at least 8 characters'); return; }
            if (password !== confirm) { showAlert('Passwords do not match'); return; }

            const btn = document.getElementById('submit-btn');
            btn.disabled = true;
            btn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Activating…';

            try {
                const r = await fetch('/api/users/accept-invite', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ token, firstName, lastName, password })
                });
                const data = await r.json().catch(() => ({}));
                if (!r.ok) throw new Error(data.message || 'Activation failed');
                show('success-view');
            } catch (err) {
                showAlert(err.message);
                btn.disabled = false;
                btn.innerHTML = '<i class="fas fa-check"></i> Activate Account';
            }
        });
    }

    // Validate token on load
    if (!token) {
        show('expired-view');
    } else {
        // Token validation happens server-side on submit; just show the form
        // (We could optionally do a GET to check token validity first)
        show('form-view');
    }
})();
