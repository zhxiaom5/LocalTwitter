import { QueryClient, QueryClientProvider, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { BrowserRouter, Navigate, NavLink, Route, Routes, useLocation, useNavigate } from 'react-router-dom';
import { ChevronsLeft, ChevronsRight, FileText, FolderHeart, Heart, Home, Info, List, LogOut, Menu, RefreshCw, Settings, UserRound, UsersRound, X } from 'lucide-react';
import { api } from './lib/api';
import { useUpdateEvents } from './lib/useUpdateEvents';
import { CreatorsPage } from './pages/CreatorsPage';
import { AboutPage } from './pages/AboutPage';
import { FavoriteCreatorsPage, FavoriteWorksPage } from './pages/FavoritesPage';
import { FeedPage } from './pages/FeedPage';
import { LoginPage } from './pages/LoginPage';
import { LogsPage } from './pages/LogsPage';
import { SettingsPage } from './pages/SettingsPage';
import { UpdatesPage } from './pages/UpdatesPage';
import { UserManagementPage } from './pages/UserManagementPage';
import type { User } from './lib/types';

export function App() {
  const [queryClient] = useState(() => createQueryClient());

  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <AppRoutes />
      </BrowserRouter>
    </QueryClientProvider>
  );
}

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        refetchOnWindowFocus: false,
      },
    },
  });
}

function AppRoutes() {
  const location = useLocation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const auth = useQuery({ queryKey: ['auth-me'], queryFn: api.me });

  useEffect(() => {
    const onUnauthorized = () => {
      queryClient.setQueryData(['auth-me'], { user: null });
      navigate('/login', { replace: true });
    };
    window.addEventListener('localtwitter:unauthorized', onUnauthorized);
    return () => window.removeEventListener('localtwitter:unauthorized', onUnauthorized);
  }, [queryClient]);

  const redirectTo = new URLSearchParams(location.search).get('redirect') || '/';
  const user = auth.data?.user ?? null;

  if (auth.isLoading) {
    return <div className="empty-state">加载登录状态...</div>;
  }

  if (!user) {
    return (
      <Routes>
        <Route path="/login" element={<LoginPage redirectTo={redirectTo === '/login' ? '/' : redirectTo} />} />
        <Route path="*" element={<Navigate to={`/login?redirect=${encodeURIComponent(location.pathname + location.search)}`} replace />} />
      </Routes>
    );
  }

  if (location.pathname === '/login') {
    return <Navigate to={redirectTo === '/login' ? '/' : redirectTo} replace />;
  }

  return <AppShell user={user} onLogout={async () => await handleLogout(queryClient, navigate)} />;
}

function AppShell({ user, onLogout }: { user: User; onLogout: () => Promise<void> }) {
  const queryClient = useQueryClient();
  useUpdateEvents();
  const superAdmin = isSuperAdminUser(user);
  const [mobileOpen, setMobileOpen] = useState(false);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [switcherOpen, setSwitcherOpen] = useState(false);
  const viewContext = useQuery({
    queryKey: ['view-context'],
    queryFn: api.viewContext,
    enabled: superAdmin,
  });
  const closeMobile = () => setMobileOpen(false);
  const sidebarToggleLabel = mobileOpen ? '关闭菜单' : sidebarCollapsed ? '展开菜单' : '隐藏菜单';
  const handleSidebarToggle = () => {
    if (mobileOpen) {
      setMobileOpen(false);
      return;
    }
    setSidebarCollapsed((current) => !current);
  };

  return (
    <div className={`app-shell ${mobileOpen ? 'mobile-nav-open' : ''} ${sidebarCollapsed ? 'sidebar-collapsed' : ''}`}>
      {!mobileOpen && (
        <button className="mobile-nav-toggle" type="button" onClick={() => setMobileOpen(true)} aria-label="打开菜单">
          <Menu size={22} />
        </button>
      )}
      <aside className="sidebar">
        <div className="sidebar-top">
          {!sidebarCollapsed && <div className="brand">LocalTwitter</div>}
          <button
            className="sidebar-collapse-toggle"
            type="button"
            onClick={handleSidebarToggle}
            aria-label={sidebarToggleLabel}
            title={sidebarToggleLabel}
          >
            {sidebarCollapsed ? <ChevronsRight size={20} /> : <ChevronsLeft size={20} />}
          </button>
        </div>
        <nav>
          <NavLink to="/" end title="首页" onClick={closeMobile}>
            <Home size={20} /> <span>首页</span>
          </NavLink>
          <NavLink to="/creators" title="关注" onClick={closeMobile}>
            <List size={20} /> <span>关注</span>
          </NavLink>
          <NavLink to="/favorite-works" title="作品收藏夹" onClick={closeMobile}>
            <Heart size={20} /> <span>作品收藏夹</span>
          </NavLink>
          <NavLink to="/favorite-creators" title="作者收藏夹" onClick={closeMobile}>
            <FolderHeart size={20} /> <span>作者收藏夹</span>
          </NavLink>
          {superAdmin && (
            <NavLink to="/users" title="用户管理" onClick={closeMobile}>
              <UsersRound size={20} /> <span>用户管理</span>
            </NavLink>
          )}
          {canUpdateUser(user) && (
            <NavLink to="/updates" title="更新管理" onClick={closeMobile}>
              <RefreshCw size={20} /> <span>更新管理</span>
            </NavLink>
          )}
          {superAdmin && (
            <NavLink to="/logs" title="日志记录" onClick={closeMobile}>
              <FileText size={20} /> <span>日志记录</span>
            </NavLink>
          )}
          <NavLink to="/settings" title="设置" onClick={closeMobile}>
            <Settings size={20} /> <span>设置</span>
          </NavLink>
          <NavLink to="/about" title="关于" onClick={closeMobile}>
            <Info size={20} /> <span>关于</span>
          </NavLink>
        </nav>
        <div className="sidebar-user">
          <div className="sidebar-user-info">
            <UserRound size={18} />
            {!sidebarCollapsed && (
              <div>
                <strong>{user.username}</strong>
                <small>{roleLabel(user)}</small>
              </div>
            )}
          </div>
          {superAdmin && !sidebarCollapsed && (
            <ViewContextControls
              context={viewContext.data ?? null}
              onOpen={() => setSwitcherOpen(true)}
              onChanged={() => {
                void viewContext.refetch();
                void queryClient.invalidateQueries();
              }}
            />
          )}
          <button className="sidebar-logout" type="button" onClick={() => void onLogout()} title="退出登录" aria-label="退出登录">
            <LogOut size={18} />
            {!sidebarCollapsed && <span>退出</span>}
          </button>
        </div>
      </aside>
      <main className="main-area">
        <Routes>
          <Route path="/" element={<FeedPage />} />
          <Route path="/creators" element={<CreatorsPage currentUser={user} />} />
          <Route path="/creator/:creatorId" element={<FeedPage />} />
          <Route path="/favorite-works" element={<FavoriteWorksPage />} />
          <Route path="/favorite-works/:folderId" element={<FavoriteWorksPage />} />
          <Route path="/favorite-works/:favoriteFolderId/play" element={<FeedPage />} />
          <Route path="/favorite-creators" element={<FavoriteCreatorsPage />} />
          <Route path="/favorite-creators/:folderId" element={<FavoriteCreatorsPage />} />
          <Route path="/updates" element={canUpdateUser(user) ? <UpdatesPage /> : <Navigate to="/" replace />} />
          <Route path="/users" element={superAdmin ? <UserManagementPage currentUser={user} onUserChange={() => void queryClient.invalidateQueries({ queryKey: ['auth-me'] })} /> : <Navigate to="/" replace />} />
          <Route path="/logs" element={superAdmin ? <LogsPage /> : <Navigate to="/" replace />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="/about" element={<AboutPage />} />
          <Route path="/login" element={<Navigate to="/" replace />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </main>
      {switcherOpen && superAdmin && (
        <ViewUserDialog
          currentUser={user}
          viewUserId={viewContext.data?.view_user?.id ?? user.id}
          onClose={() => setSwitcherOpen(false)}
          onChanged={() => {
            setSwitcherOpen(false);
            void viewContext.refetch();
            void queryClient.invalidateQueries();
          }}
        />
      )}
    </div>
  );
}

function ViewContextControls({ context, onOpen, onChanged }: { context: { actor_user: User; view_user: User } | null; onOpen: () => void; onChanged: () => void }) {
  const clear = useMutation({
    mutationFn: api.clearViewContext,
    onSuccess: onChanged,
  });
  const actor = context?.actor_user;
  const view = context?.view_user;
  const viewingOther = Boolean(actor && view && actor.id !== view.id);

  return (
    <div className="sidebar-view-context">
      {viewingOther ? <small>以 {view?.username} 身份浏览</small> : null}
      {viewingOther ? (
        <button type="button" onClick={() => clear.mutate()} disabled={clear.isPending} aria-label="切回用户">
          切回用户
        </button>
      ) : (
        <button type="button" onClick={onOpen} aria-label="切换用户">
          切换用户
        </button>
      )}
    </div>
  );
}

function ViewUserDialog({ currentUser, viewUserId, onClose, onChanged }: { currentUser: User; viewUserId: number; onClose: () => void; onChanged: () => void }) {
  const users = useQuery({ queryKey: ['users'], queryFn: api.users });
  const switchUser = useMutation({
    mutationFn: (userId: number) => api.setViewContext(userId),
    onSuccess: onChanged,
  });
  const userList = users.data?.users ?? [];

  return (
    <div className="modal-backdrop" role="dialog" aria-label="切换用户">
      <div className="modal view-user-modal">
        <div className="modal-header">
          <h2>切换用户</h2>
          <button className="modal-close" type="button" onClick={onClose} aria-label="关闭切换用户" title="关闭">
            <X size={18} />
          </button>
        </div>
        <p>以其他用户身份浏览收藏夹和收藏状态，后台权限仍保持当前超级管理员。</p>
        <div className="view-user-list">
          {userList
            .filter((item) => item.id !== currentUser.id)
            .map((item) => (
              <button key={item.id} type="button" onClick={() => switchUser.mutate(item.id)} disabled={switchUser.isPending || item.id === viewUserId} aria-label={`切换到 ${item.username}`}>
                <UserRound size={18} />
                <span>
                  <strong>{item.username}</strong>
                  <small>{roleLabel(item)}</small>
                </span>
                {item.id === viewUserId ? <em>当前</em> : null}
              </button>
            ))}
          {!users.isLoading && userList.filter((item) => item.id !== currentUser.id).length === 0 && <div className="empty-state">暂无可切换用户。</div>}
        </div>
        {switchUser.error && <p className="error-line">{switchUser.error instanceof Error ? switchUser.error.message : '切换失败'}</p>}
      </div>
    </div>
  );
}

function roleLabel(user: User) {
  if (isSuperAdminUser(user)) return '超级管理员';
  if (canUpdateUser(user) || user.role === 'admin') return '管理员';
  return '普通用户';
}

function isSuperAdminUser(user: User) {
  return Boolean(user.is_super_admin || user.role === 'super_admin');
}

function canUpdateUser(user: User) {
  return Boolean(user.can_update ?? user.is_admin);
}

async function handleLogout(queryClient: QueryClient, navigate: ReturnType<typeof useNavigate>) {
  await api.logout();
  queryClient.setQueryData(['auth-me'], { user: null });
  queryClient.removeQueries({ predicate: (query) => query.queryKey[0] !== 'auth-me' });
  navigate('/login', { replace: true });
}
