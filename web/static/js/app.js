// V2ray Collector Web App JavaScript

// Utility functions
const utility = {
    // Format numbers with commas
    formatNumber(num) {
        if (num === null || num === undefined) return '0';
        return num.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ",");
    },
    
    // Format date
    formatDate(dateString) {
        if (!dateString) return '-';
        const date = new Date(dateString);
        return date.toLocaleString('fa-IR', {
            year: 'numeric',
            month: 'long',
            day: 'numeric',
            hour: '2-digit',
            minute: '2-digit'
        });
    },
    
    // Format duration
    formatDuration(seconds) {
        if (!seconds) return '-';
        const hours = Math.floor(seconds / 3600);
        const minutes = Math.floor((seconds % 3600) / 60);
        const secs = seconds % 60;
        return `${hours}h ${minutes}m ${secs}s`;
    },
    
    // Truncate text
    truncate(text, length = 50) {
        if (!text) return '-';
        if (text.length <= length) return text;
        return text.substring(0, length) + '...';
    },
    
    // Copy to clipboard
    copyToClipboard(text) {
        navigator.clipboard.writeText(text).then(() => {
            this.showToast('کپی شد!', 'success');
        }).catch(err => {
            this.showToast('خطا در کپی', 'error');
        });
    },
    
    // Show toast notification
    showToast(message, type = 'info') {
        const toast = document.createElement('div');
        toast.className = `toast toast-${type}`;
        toast.innerHTML = `
            <span>${message}</span>
            <button onclick="this.parentElement.remove()">×</button>
        `;
        document.body.appendChild(toast);
        
        setTimeout(() => {
            toast.remove();
        }, 3000);
    },
    
    // Debounce function
    debounce(func, wait) {
        let timeout;
        return function executedFunction(...args) {
            const later = () => {
                clearTimeout(timeout);
                func(...args);
            };
            clearTimeout(timeout);
            timeout = setTimeout(later, wait);
        };
    }
};

// API functions
const api = {
    // Base URL
    baseUrl: '',
    
    // Set base URL
    setBaseUrl(url) {
        this.baseUrl = url;
    },
    
    // Fetch with error handling
    async fetch(url) {
        const response = await fetch(url);
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        return response.json();
    },
    
    // Get stats
    async getStats() {
        return this.fetch('/api/stats');
    },
    
    // Get configs
    async getConfigs(protocol = '', limit = 100, offset = 0) {
        const params = new URLSearchParams({
            protocol,
            limit,
            offset
        });
        return this.fetch(`/api/configs?${params}`);
    },
    
    // Get config details
    async getConfig(fingerprint) {
        return this.fetch(`/api/configs/${fingerprint}`);
    },
    
    // Get sites
    async getSites() {
        return this.fetch('/api/sites');
    },
    
    // Get reports
    async getReports() {
        return this.fetch('/api/reports');
    },
    
    // Test config
    async testConfig(config) {
        return fetch('/api/test', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({ config })
        }).then(response => response.json());
    }
};

// Initialize the app
document.addEventListener('DOMContentLoaded', function() {
    // Set base URL for API
    api.setBaseUrl('');
    
    // Initialize all components
    initDashboard();
    initConfigsPage();
    initTestPage();
    
    // Add global error handler
    window.addEventListener('error', function(event) {
        console.error('Error:', event.error);
        utility.showToast('خطایی رخ داد', 'error');
    });
});

// Dashboard page initialization
function initDashboard() {
    const dashboardPage = document.querySelector('.dashboard-page');
    if (!dashboardPage) return;
    
    // Load and display stats
    loadDashboardStats();
    
    // Load and display recent activity
    loadRecentActivity();
    
    // Load and display charts
    loadCharts();
}

// Load dashboard stats
async function loadDashboardStats() {
    try {
        const stats = await api.getStats();
        
        // Update stat cards
        const statCards = document.querySelectorAll('.dashboard .stat-card');
        if (statCards.length > 0) {
            statCards[0].querySelector('.stat-value').textContent = utility.formatNumber(stats.total_configs || 0);
            statCards[1].querySelector('.stat-value').textContent = utility.formatNumber(stats.valid_configs || 0);
            statCards[2].querySelector('.stat-value').textContent = utility.formatNumber(stats.working_configs || 0);
            statCards[3].querySelector('.stat-value').textContent = utility.formatNumber(stats.test_success_rate || 0) + '%';
        }
        
        // Update other elements
        const elements = {
            '.total-configs': stats.total_configs || 0,
            '.valid-configs': stats.valid_configs || 0,
            '.working-configs': stats.working_configs || 0,
            '.test-success-rate': (stats.test_success_rate || 0) + '%'
        };
        
        for (const [selector, value] of Object.entries(elements)) {
            const el = document.querySelector(selector);
            if (el) el.textContent = utility.formatNumber(value);
        }
        
    } catch (error) {
        console.error('Error loading stats:', error);
        utility.showToast('خطا در بارگذاری آمار', 'error');
    }
}

// Load recent activity
async function loadRecentActivity() {
    try {
        const reports = await api.getReports();
        const activityContainer = document.querySelector('.recent-activity');
        if (!activityContainer) return;
        
        if (!reports || reports.length === 0) {
            activityContainer.innerHTML = '<p>هیچ فعالیت اخیری یافت نشد</p>';
            return;
        }
        
        // Sort by date (newest first)
        reports.sort((a, b) => new Date(b.mod_time) - new Date(a.mod_time));
        
        // Take first 5
        const recentReports = reports.slice(0, 5);
        
        let html = '<ul>';
        recentReports.forEach(report => {
            const date = utility.formatDate(report.mod_time);
            const name = utility.truncate(report.name, 30);
            html += `
                <li>
                    <span class="activity-date">${date}</span>
                    <span class="activity-name">${name}</span>
                </li>
            `;
        });
        html += '</ul>';
        
        activityContainer.innerHTML = html;
        
    } catch (error) {
        console.error('Error loading recent activity:', error);
    }
}

// Load charts
async function loadCharts() {
    try {
        const stats = await api.getStats();
        
        // Protocol distribution chart
        if (stats.protocol_distribution) {
            renderProtocolChart(stats.protocol_distribution);
        }
        
        // Site accessibility chart
        if (stats.site_accessibility) {
            renderSiteChart(stats.site_accessibility);
        }
        
    } catch (error) {
        console.error('Error loading charts:', error);
    }
}

// Render protocol distribution chart
function renderProtocolChart(data) {
    const container = document.getElementById('protocol-chart');
    if (!container) return;
    
    // Simple bar chart
    let html = '<div class="chart">';
    const max = Math.max(...Object.values(data));
    
    for (const [protocol, count] of Object.entries(data)) {
        const percentage = (count / max) * 100;
        html += `
            <div class="chart-bar">
                <div class="chart-label">${protocol}</div>
                <div class="chart-bar-fill" style="width: ${percentage}%">
                    <span class="chart-value">${utility.formatNumber(count)}</span>
                </div>
            </div>
        `;
    }
    html += '</div>';
    
    container.innerHTML = html;
}

// Render site accessibility chart
function renderSiteChart(data) {
    const container = document.getElementById('site-chart');
    if (!container) return;
    
    // Simple table chart
    let html = '<table class="chart-table"><thead><tr><th>سایت</th><th>دسترسی</th><th>موفقیت</th></tr></thead><tbody>';
    
    for (const [site, info] of Object.entries(data)) {
        const successRate = ((info.success || 0) / (info.tested || 1)) * 100;
        html += `
            <tr>
                <td>${site}</td>
                <td>${utility.formatNumber(info.tested || 0)}</td>
                <td>
                    <div class="progress-bar">
                        <div class="progress-fill" style="width: ${successRate}%">
                            ${successRate.toFixed(1)}%
                        </div>
                    </div>
                </td>
            </tr>
        `;
    }
    html += '</tbody></table>';
    
    container.innerHTML = html;
}

// Configs page initialization
function initConfigsPage() {
    const configsPage = document.querySelector('.configs-page');
    if (!configsPage) return;
    
    // Load configs
    loadConfigs();
    
    // Setup filters
    setupConfigFilters();
    
    // Setup search
    setupConfigSearch();
}

// Load configs
async function loadConfigs(protocol = '', limit = 100, offset = 0) {
    try {
        const configs = await api.getConfigs(protocol, limit, offset);
        renderConfigs(configs);
    } catch (error) {
        console.error('Error loading configs:', error);
        utility.showToast('خطا در بارگذاری کانفیگ‌ها', 'error');
    }
}

// Render configs
function renderConfigs(configs) {
    const container = document.querySelector('.configs-table tbody');
    if (!container) return;
    
    if (!configs || configs.length === 0) {
        container.innerHTML = '<tr><td colspan="5" class="text-center">هیچ کانفیگی یافت نشد</td></tr>';
        return;
    }
    
    let html = '';
    configs.forEach(config => {
        const shortValue = utility.truncate(config.value, 40);
        const firstSeen = utility.formatDate(config.first_seen);
        const lastSeen = utility.formatDate(config.last_seen);
        
        html += `
            <tr>
                <td>${shortValue}</td>
                <td><span class="badge badge-info">${config.protocol}</span></td>
                <td>${firstSeen}</td>
                <td>${lastSeen}</td>
                <td>
                    <button class="btn btn-small" onclick="copyConfig('${config.value}')">کپی</button>
                    <a href="/configs/${config.fingerprint}" class="btn btn-small btn-secondary">جزئیات</a>
                </td>
            </tr>
        `;
    });
    
    container.innerHTML = html;
}

// Copy config to clipboard
function copyConfig(config) {
    utility.copyToClipboard(config);
}

// Setup config filters
function setupConfigFilters() {
    const protocolSelect = document.getElementById('protocol-filter');
    if (!protocolSelect) return;
    
    protocolSelect.addEventListener('change', function() {
        loadConfigs(this.value, 100, 0);
    });
}

// Setup config search
function setupConfigSearch() {
    const searchInput = document.getElementById('config-search');
    if (!searchInput) return;
    
    const debouncedSearch = utility.debounce(function() {
        // Filter configs based on search
        const searchTerm = this.value.toLowerCase();
        const rows = document.querySelectorAll('.configs-table tbody tr');
        
        rows.forEach(row => {
            const text = row.textContent.toLowerCase();
            row.style.display = text.includes(searchTerm) ? '' : 'none';
        });
    }, 300);
    
    searchInput.addEventListener('input', debouncedSearch);
}

// Test page initialization
function initTestPage() {
    const testPage = document.querySelector('.test-page');
    if (!testPage) return;
    
    // Setup test form
    const testForm = document.getElementById('test-form');
    if (testForm) {
        testForm.addEventListener('submit', function(e) {
            e.preventDefault();
            testConfig();
        });
    }
    
    // Load sites for dropdown
    loadSites();
}

// Test config
async function testConfig() {
    const configInput = document.getElementById('config-input');
    const resultsContainer = document.getElementById('test-results');
    const loadingIndicator = document.getElementById('test-loading');
    
    if (!configInput || !resultsContainer) return;
    
    const config = configInput.value.trim();
    if (!config) {
        utility.showToast('لطفاً کانفیگ را وارد کنید', 'warning');
        return;
    }
    
    // Show loading
    if (loadingIndicator) loadingIndicator.style.display = 'block';
    resultsContainer.innerHTML = '';
    
    try {
        const result = await api.testConfig(config);
        renderTestResults(result);
    } catch (error) {
        utility.showToast('خطا در تست کانفیگ', 'error');
        resultsContainer.innerHTML = '<p class="text-danger">خطا در تست کانفیگ</p>';
    } finally {
        if (loadingIndicator) loadingIndicator.style.display = 'none';
    }
}

// Render test results
function renderTestResults(result) {
    const container = document.getElementById('test-results');
    if (!container) return;
    
    if (!result || !result.site_results) {
        container.innerHTML = '<p>هیچ نتیجه‌ای یافت نشد</p>';
        return;
    }
    
    let html = '<div class="test-results">';
    html += `<p><strong>کانفیگ:</strong> ${utility.truncate(result.config, 50)}</p>`;
    html += `<p><strong>معتبر:</strong> ${result.valid ? '✅ بله' : '❌ خیر'}</p>`;
    
    if (result.error) {
        html += `<p class="text-danger">خطا: ${result.error}</p>`;
    } else {
        html += '<p><strong>نتایج تست سایت‌ها:</strong></p>';
        html += '<table class="test-results-table"><thead><tr><th>سایت</th><th>وضعیت</th><th>تاخیر</th></tr></thead><tbody>';
        
        for (const [site, info] of Object.entries(result.site_results)) {
            const status = info.success ? '✅ موفق' : '❌ ناموفق';
            const latency = info.latency ? `${Math.round(info.latency / 1000000)}ms` : '-';
            html += `
                <tr>
                    <td>${site}</td>
                    <td>${status}</td>
                    <td>${latency}</td>
                </tr>
            `;
        }
        
        html += '</tbody></table>';
        html += `<p><strong>موفقیت:</strong> ${result.total_success || 0}/${result.total_tested || 0} سایت</p>`;
    }
    
    html += '</div>';
    container.innerHTML = html;
}

// Load sites for dropdown
async function loadSites() {
    try {
        const sites = await api.getSites();
        const select = document.getElementById('site-select');
        if (!select) return;
        
        sites.forEach(site => {
            const option = document.createElement('option');
            option.value = site.url;
            option.textContent = site.name;
            select.appendChild(option);
        });
    } catch (error) {
        console.error('Error loading sites:', error);
    }
}

// Add CSS for toast notifications
const toastStyle = document.createElement('style');
toastStyle.textContent = `
    .toast {
        position: fixed;
        bottom: 20px;
        right: 20px;
        padding: 1rem 1.5rem;
        border-radius: 8px;
        background: #1e293b;
        color: white;
        box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
        z-index: 1000;
        display: flex;
        align-items: center;
        gap: 1rem;
        animation: slideIn 0.3s ease;
    }
    
    .toast button {
        background: none;
        border: none;
        color: white;
        cursor: pointer;
        font-size: 1.2rem;
        opacity: 0.7;
    }
    
    .toast button:hover {
        opacity: 1;
    }
    
    .toast-success {
        border-left: 4px solid #22c55e;
    }
    
    .toast-warning {
        border-left: 4px solid #f59e0b;
    }
    
    .toast-danger {
        border-left: 4px solid #ef4444;
    }
    
    .toast-info {
        border-left: 4px solid #6366f1;
    }
    
    @keyframes slideIn {
        from {
            transform: translateX(100%);
            opacity: 0;
        }
        to {
            transform: translateX(0);
            opacity: 1;
        }
    }
    
    .progress-bar {
        width: 100%;
        height: 20px;
        background: var(--card-bg);
        border-radius: 10px;
        overflow: hidden;
    }
    
    .progress-fill {
        height: 100%;
        background: linear-gradient(90deg, var(--primary-color), var(--success-color));
        border-radius: 10px;
        display: flex;
        align-items: center;
        justify-content: flex-end;
        padding-right: 5px;
        color: white;
        font-size: 0.75rem;
        font-weight: 600;
    }
    
    .chart {
        width: 100%;
    }
    
    .chart-bar {
        margin-bottom: 0.5rem;
        display: flex;
        align-items: center;
        gap: 1rem;
    }
    
    .chart-label {
        width: 80px;
        font-size: 0.875rem;
        color: var(--text-secondary);
    }
    
    .chart-bar-fill {
        height: 20px;
        background: var(--primary-color);
        border-radius: 4px;
        position: relative;
        flex: 1;
    }
    
    .chart-value {
        position: absolute;
        right: 5px;
        top: 50%;
        transform: translateY(-50%);
        font-size: 0.75rem;
        color: white;
    }
    
    .chart-table {
        width: 100%;
        border-collapse: collapse;
    }
    
    .chart-table th,
    .chart-table td {
        padding: 0.5rem;
        text-align: right;
        border-bottom: 1px solid var(--card-border);
    }
    
    .btn-small {
        padding: 0.25rem 0.75rem;
        font-size: 0.8rem;
    }
    
    .text-center {
        text-align: center !important;
    }
    
    .text-danger {
        color: var(--danger-color) !important;
    }
    
    .text-success {
        color: var(--success-color) !important;
    }
    
    .recent-activity ul {
        list-style: none;
        padding: 0;
        margin: 0;
    }
    
    .recent-activity li {
        padding: 0.5rem 0;
        border-bottom: 1px solid var(--card-border);
        display: flex;
        justify-content: space-between;
    }
    
    .activity-date {
        color: var(--text-secondary);
        font-size: 0.875rem;
    }
    
    .test-results-table {
        width: 100%;
        margin-top: 1rem;
    }
`;
document.head.appendChild(toastStyle);
