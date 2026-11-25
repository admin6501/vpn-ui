async function panelActivityWrite(path, data) {
    try {
        const response = await fetch(basePath + 'panel/api/activity/' + path, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', 'X-Requested-With': 'XMLHttpRequest' },
            body: JSON.stringify(data),
        });
        const result = await response.json();
        if (result.msg) Vue.prototype.$message[result.success ? 'success' : 'error'](result.msg);
        return result.success === true;
    } catch (error) {
        Vue.prototype.$message.error(String(error));
        return false;
    }
}

Vue.component('panel-activity', {
    template: '#panel-activity-template',
    data: () => ({ basePath, notices: {} }),
    methods: {
        async refresh() {
            if (document.hidden || this.pending) return;
            this.pending = true;
            try {
                // A background poll should not turn a network outage into repeated toasts.
                const { data } = await axios.get('/panel/api/activity/notices');
                if (data.success) this.notices = data.obj;
            } catch (error) {
                if (error.response?.status === 404) clearInterval(this.timer);
            } finally {
                this.pending = false;
            }
        },
        async dismiss(kind) {
            const through = this.notices[kind]?.through;
            if (through && await panelActivityWrite('notices/dismiss', { kind, through })) {
                if (this.notices[kind]?.through === through) this.$delete(this.notices, kind);
                await this.refresh();
            }
        },
    },
    mounted() {
        this.refresh();
        this.timer = setInterval(this.refresh, 20000);
        document.addEventListener('visibilitychange', this.refresh);
        window.addEventListener('panel-activity-refresh', this.refresh);
    },
    beforeDestroy() {
        clearInterval(this.timer);
        document.removeEventListener('visibilitychange', this.refresh);
        window.removeEventListener('panel-activity-refresh', this.refresh);
    },
});
