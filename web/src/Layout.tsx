import { useEffect, useState } from "react";
import {
  Avatar,
  Box,
  Divider,
  Drawer,
  IconButton,
  List,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Menu,
  MenuItem,
  Tooltip,
  Typography,
  useMediaQuery,
  useTheme,
} from "@mui/material";
import HubRoundedIcon from "@mui/icons-material/HubRounded";
import PeopleAltRoundedIcon from "@mui/icons-material/PeopleAltRounded";
import AutoStoriesRoundedIcon from "@mui/icons-material/AutoStoriesRounded";
import AutoAwesomeRoundedIcon from "@mui/icons-material/AutoAwesomeRounded";
import FactCheckRoundedIcon from "@mui/icons-material/FactCheckRounded";
import AdminPanelSettingsRoundedIcon from "@mui/icons-material/AdminPanelSettingsRounded";
import TuneRoundedIcon from "@mui/icons-material/TuneRounded";
import MenuRoundedIcon from "@mui/icons-material/MenuRounded";
import LogoutRoundedIcon from "@mui/icons-material/LogoutRounded";
import InfoOutlinedIcon from "@mui/icons-material/InfoOutlined";
import { Link, Outlet, useLocation, useNavigate } from "react-router-dom";
import { useAuth } from "./AuthContext";
import { Brand } from "./components/Brand";
import { api } from "./api";

const drawerWidth = 264;
const nav = [
  { path: "/orbit", label: "나의 우주", icon: <HubRoundedIcon /> },
  { path: "/people", label: "관계", icon: <PeopleAltRoundedIcon /> },
  { path: "/memories", label: "기억", icon: <AutoStoriesRoundedIcon /> },
  { path: "/ai", label: "Orbit AI", icon: <AutoAwesomeRoundedIcon /> },
  { path: "/approvals", label: "검토 및 승인", icon: <FactCheckRoundedIcon /> },
];

export function Layout() {
  const { user, config, logout } = useAuth();
  const location = useLocation();
  const navigate = useNavigate();
  const theme = useTheme();
  const desktop = useMediaQuery(theme.breakpoints.up("md"));
  const [mobileOpen, setMobileOpen] = useState(false);
  const [profileAnchor, setProfileAnchor] = useState<HTMLElement | null>(null);
  const [approvalEnabled, setApprovalEnabled] = useState(false);

  useEffect(() => {
    localStorage.setItem("orbit:last-route", location.pathname);
  }, [location.pathname]);
  useEffect(() => {
    api<{ enabled: boolean }>("/approvals")
      .then((v) => setApprovalEnabled(v.enabled))
      .catch(() => undefined);
  }, [location.pathname]);
  const coreItems = nav.filter(
    (item) => item.path !== "/approvals" || approvalEnabled,
  );
  const settingItems = [
    { path: "/personal", label: "개인화 및 키", icon: <TuneRoundedIcon /> },
    ...(user?.role === "admin"
      ? [
          {
            path: "/admin",
            label: "서비스 관리",
            icon: <AdminPanelSettingsRoundedIcon />,
          },
        ]
      : []),
  ];
  const drawer = (
    <Box
      sx={{
        height: "100%",
        display: "flex",
        flexDirection: "column",
        bgcolor: "rgba(9,11,24,.88)",
        backdropFilter: "blur(18px)",
      }}
    >
      <Box sx={{ p: 3, pb: 2.5 }}>
        <Brand />
      </Box>
      <Typography
        variant="overline"
        color="text.secondary"
        sx={{ px: 3, mt: 1, letterSpacing: ".14em" }}
      >
        MY UNIVERSE
      </Typography>
      <List sx={{ px: 1.5, py: 1 }}>
        {coreItems.map((item) => (
          <NavItem
            key={item.path}
            item={item}
            active={location.pathname.startsWith(item.path)}
            close={() => setMobileOpen(false)}
          />
        ))}
      </List>
      <Divider sx={{ mx: 2, my: 1 }} />
      <Typography
        variant="overline"
        color="text.secondary"
        sx={{ px: 3, mt: 1, letterSpacing: ".14em" }}
      >
        SETTINGS
      </Typography>
      <List sx={{ px: 1.5, py: 1 }}>
        {settingItems.map((item) => (
          <NavItem
            key={item.path}
            item={item}
            active={location.pathname.startsWith(item.path)}
            close={() => setMobileOpen(false)}
          />
        ))}
      </List>
      <Box sx={{ mt: "auto", p: 2 }}>
        <ListItemButton
          onClick={(event) => setProfileAnchor(event.currentTarget)}
          sx={{
            borderRadius: 3,
            p: 1.2,
            border: "1px solid",
            borderColor: "divider",
          }}
        >
          <Avatar
            sx={{ width: 38, height: 38, bgcolor: "primary.dark", mr: 1.3 }}
          >
            {user?.display_name?.slice(0, 1)}
          </Avatar>
          <ListItemText
            primary={user?.display_name}
            secondary={
              user?.role === "admin"
                ? "서비스 관리자"
                : user?.role === "team_lead"
                  ? "팀장"
                  : "멤버"
            }
            slotProps={{ primary: { sx: { fontWeight: 700 } } }}
          />
        </ListItemButton>
      </Box>
    </Box>
  );
  return (
    <Box
      sx={{
        minHeight: "100vh",
        display: "flex",
        background:
          "radial-gradient(circle at 85% 5%,rgba(91,72,173,.12),transparent 25%),#090b18",
      }}
    >
      {!desktop && (
        <Tooltip title="메뉴">
          <IconButton
            onClick={() => setMobileOpen(true)}
            sx={{
              position: "fixed",
              top: 14,
              left: 14,
              zIndex: 1100,
              bgcolor: "background.paper",
              border: "1px solid",
              borderColor: "divider",
            }}
          >
            <MenuRoundedIcon />
          </IconButton>
        </Tooltip>
      )}
      <Drawer
        variant={desktop ? "permanent" : "temporary"}
        open={desktop || mobileOpen}
        onClose={() => setMobileOpen(false)}
        ModalProps={{ keepMounted: true }}
        sx={{
          width: desktop ? drawerWidth : 0,
          flexShrink: 0,
          "& .MuiDrawer-paper": {
            width: drawerWidth,
            boxSizing: "border-box",
            borderRightColor: "divider",
          },
        }}
      >
        {drawer}
      </Drawer>
      <Box
        component="main"
        sx={{
          width: { xs: "100%", md: `calc(100% - ${drawerWidth}px)` },
          minWidth: 0,
          p: {
            xs: "76px 18px 36px",
            sm: "84px 28px 48px",
            md: "40px clamp(30px,4vw,64px) 64px",
          },
        }}
      >
        <Outlet />
      </Box>
      <Menu
        anchorEl={profileAnchor}
        open={Boolean(profileAnchor)}
        onClose={() => setProfileAnchor(null)}
        slotProps={{
          paper: {
            sx: {
              width: 270,
              mt: 1,
              maxHeight: 360,
              overflowY: "auto",
              backgroundImage:
                "linear-gradient(150deg,rgba(34,31,61,.98),rgba(17,20,38,.98))",
              border: "1px solid",
              borderColor: "divider",
            },
          },
        }}
      >
        <Box sx={{ px: 2, py: 1.5 }}>
          <Typography sx={{ fontWeight: 750 }}>{user?.display_name}</Typography>
          <Typography variant="body2" color="text.secondary">
            {user?.username}
          </Typography>
        </Box>
        <Divider />
        <MenuItem
          onClick={() => {
            navigate("/personal");
            setProfileAnchor(null);
          }}
        >
          <TuneRoundedIcon fontSize="small" sx={{ mr: 1.5 }} />
          개인화 설정
        </MenuItem>
        {user?.role === "admin" && (
          <MenuItem
            onClick={() => {
              navigate("/admin");
              setProfileAnchor(null);
            }}
          >
            <AdminPanelSettingsRoundedIcon fontSize="small" sx={{ mr: 1.5 }} />
            서비스 관리
          </MenuItem>
        )}
        <Divider />
        <MenuItem disabled>
          <InfoOutlinedIcon fontSize="small" sx={{ mr: 1.5 }} />
          Orbit {config?.version ?? "dev"}
        </MenuItem>
        <MenuItem
          onClick={async () => {
            await logout();
            navigate("/login");
          }}
        >
          <LogoutRoundedIcon fontSize="small" sx={{ mr: 1.5 }} />
          로그아웃
        </MenuItem>
      </Menu>
    </Box>
  );
}

function NavItem({
  item,
  active,
  close,
}: {
  item: (typeof nav)[number];
  active: boolean;
  close: () => void;
}) {
  return (
    <ListItemButton
      component={Link}
      to={item.path}
      onClick={close}
      selected={active}
      sx={{
        borderRadius: 2.5,
        mb: 0.5,
        minHeight: 48,
        "&.Mui-selected": {
          bgcolor: "rgba(169,155,248,.13)",
          color: "primary.light",
          "&::before": {
            content: '""',
            width: 3,
            height: 24,
            bgcolor: "primary.main",
            borderRadius: 2,
            position: "absolute",
            left: 0,
          },
        },
      }}
    >
      <ListItemIcon sx={{ minWidth: 42, color: "inherit" }}>
        {item.icon}
      </ListItemIcon>
      <ListItemText
        primary={item.label}
        slotProps={{ primary: { sx: { fontWeight: active ? 720 : 580 } } }}
      />
    </ListItemButton>
  );
}
