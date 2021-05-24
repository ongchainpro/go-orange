Pod::Spec.new do |spec|
  spec.name         = 'Gong'
  spec.version      = '{{.Version}}'
  spec.license      = { :type => 'GNU Lesser General Public License, Version 3.0' }
  spec.homepage     = 'https://github.com/ong2020/go-orange'
  spec.authors      = { {{range .Contributors}}
		'{{.Name}}' => '{{.Email}}',{{end}}
	}
  spec.summary      = 'iOS Orange Client'
  spec.source       = { :git => 'https://github.com/ong2020/go-orange.git', :commit => '{{.Commit}}' }

	spec.platform = :ios
  spec.ios.deployment_target  = '9.0'
	spec.ios.vendored_frameworks = 'Frameworks/Gong.framework'

	spec.prepare_command = <<-CMD
    curl https://gongstore.blob.core.windows.net/builds/{{.Archive}}.tar.gz | tar -xvz
    mkdir Frameworks
    mv {{.Archive}}/Gong.framework Frameworks
    rm -rf {{.Archive}}
  CMD
end
