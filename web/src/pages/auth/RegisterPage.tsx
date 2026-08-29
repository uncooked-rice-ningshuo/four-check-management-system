import { LockOutlined, UserOutlined } from "@ant-design/icons";
import { useQuery } from "@tanstack/react-query";
import { Alert, Button, Checkbox, Form, Input, Select, Typography, message } from "antd";
import { Link, useNavigate } from "react-router-dom";
import { registerAccount } from "../../shared/api/authApi";
import { getRegistrationOptions } from "../../shared/api/adminApi";
import { AuthLayout } from "../../shared/layouts/AuthLayout";

export function RegisterPage() {
  const navigate = useNavigate();
  const options = useQuery({ queryKey: ["registration-options"], queryFn: getRegistrationOptions });
  const submit = async (values: {
    studentNo: string;
    name: string;
    password: string;
    confirmPassword: string;
    requestedDepartmentId: string;
    requestedOrgRoleId: string;
  }) => {
    try {
      await registerAccount({
        studentNo: values.studentNo,
        name: values.name,
        password: values.password,
        requestedDepartmentId: values.requestedDepartmentId,
        requestedOrgRoleId: values.requestedOrgRoleId,
      });
      navigate("/register/result");
    } catch (error) {
      message.error((error as Error).message);
    }
  };
  return (
    <AuthLayout variant="register">
      <div className="auth-form compact">
        <Typography.Title level={1}>注册系统账号</Typography.Title>
        <Typography.Paragraph className="subtitle">
          请如实填写身份信息
        </Typography.Paragraph>
        <Form
          layout="vertical"
          size="large"
          onFinish={submit}
          requiredMark={false}
        >
          <Form.Item
            label="学号"
            name="studentNo"
            rules={[{ required: true, message: "请输入学号" }]}
          >
            <Input prefix={<UserOutlined />} placeholder="请输入您的学号…" />
          </Form.Item>
          <Form.Item label="申请部门" name="requestedDepartmentId" rules={[{ required: true, message: "请选择申请部门" }]}>
            <Select loading={options.isLoading} placeholder="请选择部门…" options={options.data?.departments.map(item => ({ value: item.id, label: item.name }))} />
          </Form.Item>
          <Form.Item label="申请角色" name="requestedOrgRoleId" rules={[{ required: true, message: "请选择申请角色" }]}>
            <Select loading={options.isLoading} placeholder="请选择组织角色…" options={options.data?.organizationRoles.map(item => ({ value: item.id, label: item.name }))} />
          </Form.Item>
          <Form.Item
            label="姓名"
            name="name"
            rules={[{ required: true, message: "请输入姓名" }]}
          >
            <Input prefix={<UserOutlined />} placeholder="请输入您的真实姓名…" />
          </Form.Item>
          <Form.Item
            label="密码"
            name="password"
            rules={[
              { required: true, message: "请输入密码" },
              { min: 8, message: "密码至少 8 位" },
            ]}
          >
            <Input.Password
              prefix={<LockOutlined />}
              placeholder="请设置登录密码…"
            />
          </Form.Item>
          <Form.Item
            label="确认密码"
            name="confirmPassword"
            dependencies={["password"]}
            rules={[
              { required: true, message: "请再次输入密码" },
              ({ getFieldValue }) => ({
                validator(_, value) {
                  return !value || getFieldValue("password") === value
                    ? Promise.resolve()
                    : Promise.reject(new Error("两次输入的密码不一致"));
                },
              }),
            ]}
          >
            <Input.Password
              prefix={<LockOutlined />}
              placeholder="请再次输入登录密码…"
            />
          </Form.Item>
          <Form.Item
            name="agreement"
            valuePropName="checked"
            rules={[
              {
                validator: (_, value) =>
                  value
                    ? Promise.resolve()
                    : Promise.reject(new Error("请确认信息真实准确")),
              },
            ]}
          >
            <Checkbox>我已确认填写的信息真实准确，愿意遵守系统使用规范</Checkbox>
          </Form.Item>
          <Button
            type="primary"
            htmlType="submit"
            block
            className="primary-button"
          >
            注册账号
          </Button>
        </Form>
        <p className="switch">
          已有账号？ <Link to="/login">立即登录</Link>
        </p>
        <Alert
          type="info"
          showIcon
          title="提交后账号进入待审核状态，审核通过后方可登录。"
        />
      </div>
    </AuthLayout>
  );
}
