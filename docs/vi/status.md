# Trạng thái hiện tại

[English](../status.md) · **Tiếng Việt**

Modex `v0.1.0-dev.38` là bản xem trước dành cho phát triển, chưa sẵn sàng cho production. Package Android là `dev.modex`, hỗ trợ Android 8.0 (API 26) trở lên.

| Hạng mục | Trạng thái hiện tại |
| --- | --- |
| Ứng dụng Android | Ứng dụng Jetpack Compose native, phân phối dưới dạng APK pre-release phát triển. |
| Host | Thao tác native trên Linux và macOS; dùng Windows làm host cần WSL2. |
| Truyền tải | Kết nối SSH tới host supervisor và journal sự kiện có giới hạn. |
| Agent | Codex là đường tích hợp đầy đủ nhất. OpenCode và Antigravity có phạm vi hẹp hơn, được ghi rõ. |
| Thông tin xác thực | Thông tin xác thực provider và quá trình chạy agent ở trên host. |
| Cloud và điều phối | Chỉ là định hướng tương lai, chưa có trong bản xem trước. |
| Giấy phép | Chưa chọn giấy phép dự án. |

Xem [hướng dẫn cài đặt](../install.md) để biết cách tải và nền tảng hiện hỗ trợ, cùng [ma trận hỗ trợ agent](agents.md) để biết giới hạn theo khả năng. Không xem bản phát triển là kênh ký phát hành hoặc hỗ trợ production.

## Đã hoạt động trong bản xem trước

- Chat Android native qua SSH với journal host có thể kết nối lại.
- Hội thoại Codex, streaming, lịch sử, phê duyệt/câu hỏi được hỗ trợ, tệp đính kèm, điều khiển plan và hoạt động subagent chỉ đọc.
- Hội thoại OpenCode, model, project, form, trạng thái công cụ và lịch sử trong các giới hạn đã ghi rõ.
- Hội thoại văn bản Antigravity, streaming, Stop, lịch sử và quyền dùng một lần với model mặc định.

## Đang tập trung

- Làm OpenCode và Antigravity ổn định hơn với CLI native, đồng thời thu hẹp các khoảng trống tính năng có giá trị cao nhất.
- Cải thiện cài đặt, thông báo khôi phục, UX trên thiết bị thật, khả năng tiếp cận và kiểm chứng release.
- Giữ mô hình khả năng độc lập với agent để một kết nối host không khóa toàn bộ chat vào một agent.

## Chuẩn bị tiếp theo

- Thông báo hoàn thành nền và yêu cầu hành động đáng tin cậy hơn.
- Mở rộng cài đặt và vòng đời host, gồm host Windows native khi cơ chế bảo vệ trạng thái riêng tư đã sẵn sàng.
- Nền tảng relay và môi trường thực thi được quản lý trong [lộ trình](roadmap.md); các phần này chưa có trong bản xem trước hiện tại.
