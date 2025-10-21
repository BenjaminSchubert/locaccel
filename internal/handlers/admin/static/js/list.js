document.querySelector('#cacheEntries tbody').addEventListener('click', async (e) => {
    const button = e.target.closest('.deleteButton');
    if (!button) {
        return;
    }

    const row = button.closest('tr')
    const key = row.dataset.id;

    if (!confirm(`Delete entry ${key}?`)) {
        return;
    }

    try {
        const resp = await fetch(`/cache/${encodeURIComponent(key)}`, { method: 'DELETE' });
        if (!resp.ok) {
            throw new Error(`server returned: ${resp.status}`);
        }
        row.remove()
    } catch (err) {
        console.error(err);
        alert(`Could not delete entry: ${err}`)
    }
});
