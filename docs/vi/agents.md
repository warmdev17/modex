# Hỗ trợ agent

[English](../agents.md) · **Tiếng Việt**

Phạm vi hỗ trợ khác nhau theo từng runtime. Modex không tuyên bố các agent có bộ tính năng tương đương. Hãy cài và đăng nhập runtime trên host trước khi dùng; bridge đang chạy không chứng minh provider đã xác thực hay inference hoạt động.

## Kết nối agent

1. Cài runtime trên host và đăng nhập provider theo quy trình bình thường của runtime đó.
2. Cài Modex rồi khởi động host service cho workspace. Service mặc định dùng Codex; OpenCode cần cấu hình backend riêng theo [hướng dẫn backend](../opencode-backend.md).
3. Bật đăng nhập SSH bằng public key, kiểm tra fingerprint của host key bên ngoài Modex, sau đó thêm host vào ứng dụng Android.
4. Mở hoặc tạo project rồi chọn runtime khi tạo chat mới. Antigravity chỉ xuất hiện trên kết nối dùng Codex sau khi host xác nhận CLI, đăng nhập và khả năng khôi phục phiên ACP đã sẵn sàng.

Chat hiện có tiếp tục dùng runtime và phiên native đã chọn. Kết nối host không khóa toàn bộ project hay chat vào một agent.

| Runtime | Có trong bản xem trước này | Giới hạn đã biết |
| --- | --- | --- |
| Codex | Đường tích hợp đầy đủ nhất: chat, streaming, lịch sử, tệp đính kèm, phê duyệt/câu hỏi được hỗ trợ và các điều khiển riêng Codex mà host công bố. | Một số khả năng và thao tác native của Codex chưa có; xem [tài liệu protocol](../history-contract.md) và [thiết kế](../design.md). |
| OpenCode | Chat, lịch sử, chọn model, tệp đính kèm, ngữ cảnh project, form và trạng thái công cụ. | Chưa có plan mode, permission profile, takeover, reference hay toàn bộ điều khiển project riêng Codex. Trạng thái phiên chỉ tồn tại theo supervisor. |
| Antigravity | Prompt văn bản, model mặc định, streaming, Stop, lịch sử và quyền dùng một lần. | Không có tệp đính kèm hoặc steer; không chọn được model và không có đầy đủ điều khiển như Codex/OpenCode. |

Chỉ có executable không đồng nghĩa agent đã sẵn sàng. Hiện tại, bộ kiểm tra Antigravity xác nhận CLI, đăng nhập và các khả năng ACP bắt buộc trước khi cho phép chọn. Host Codex và OpenCode khởi động thành công chỉ chứng minh bridge đã khởi tạo, không chứng minh provider đã xác thực hay inference sẽ thành công. Tài khoản provider và thông tin xác thực được cấu hình, kiểm tra trên host.

Ma trận này mô tả bản xem trước hiện tại. [Snapshot triển khai backend OpenCode](../opencode-backend.md) ghi lại một revision đã kiểm chứng trước đây và có thể chưa gồm phần project, form hay lịch sử tool mới hơn.

## Antigravity trong bản phát triển

Khi cả host và Android hỗ trợ tùy chọn phiên Antigravity, bộ chọn trong
chat hiển thị các model và mức suy nghĩ do ACP cung cấp. Có thể đổi khi chat
đang rảnh. Modex lưu lựa chọn đã được xác nhận theo từng chat và đối chiếu
với các tùy chọn còn khả dụng khi mở lại. Host cũ và phiên ACP thiếu tùy
chọn tiếp tục dùng model mặc định của Antigravity.
Antigravity 1.5.17 đưa chat mới vào danh sách khôi phục sau lượt chat đầu
tiên được ghi lại.

Bảng Agents hiển thị lượt gọi subagent do Antigravity báo về. Trạng thái
hoàn tất của lượt gọi nghĩa là công cụ đã trả kết quả. Bản CLI được kiểm
thử cung cấp trạng thái lượt gọi, còn lịch sử riêng của subagent chưa có.
